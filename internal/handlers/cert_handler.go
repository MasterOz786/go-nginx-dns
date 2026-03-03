package handlers

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/gin-gonic/gin"
)

type CertRequest struct {
	Domain      string `json:"domain" binding:"required"`
	Email       string `json:"email" binding:"required"`
	CertbotMode string `json:"mode"` // "standalone" or "webroot"
	WebrootPath string `json:"webroot_path,omitempty"`
	DryRun      bool   `json:"dry_run"`
	Staging     bool   `json:"staging"` // Use staging server for testing
}

type CertResponse struct {
	Status     string   `json:"status"`
	Message    string   `json:"message"`
	Domain     string   `json:"domain"`
	CertPath   string   `json:"cert_path,omitempty"`
	KeyPath    string   `json:"key_path,omitempty"`
	Fullchain  string   `json:"fullchain,omitempty"`
	ExpiryDate string   `json:"expiry_date,omitempty"`
	Commands   []string `json:"commands,omitempty"`
	Error      string   `json:"error,omitempty"`
}

func GenerateCertbotCert(c *gin.Context) {
	var req struct {
		Domain string `json:"domain"`
		Email  string `json:"email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"status": "error", "error": err.Error()})
		return
	}

	if req.Domain == "" {
		c.JSON(400, gin.H{"status": "error", "error": "domain is required"})
		return
	}

	if req.Email == "" {
		req.Email = "admin@" + req.Domain
	}

	cmd := exec.Command(
		"sudo",
		"certbot",
		"--nginx",
		"--non-interactive",
		"--agree-tos",
		"-m", req.Email,
		"-d", req.Domain,
		"-d", "www."+req.Domain,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		c.JSON(500, gin.H{
			"status": "error",
			"error":  string(output),
		})
		return
	}

	c.JSON(200, gin.H{
		"status":  "success",
		"message": "Certificate generated successfully",
		"output":  string(output),
	})
}

func RenewCertbotCert(c *gin.Context) {
	domain := c.Query("domain")
	if domain == "" {
		c.JSON(400, gin.H{"status": "error", "error": "domain parameter required"})
		return
	}

	args := []string{"renew", "--cert-name", domain, "--non-interactive"}

	cmd := exec.Command("certbot", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		c.JSON(500, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Renewal failed: %v\nOutput: %s", err, string(output)),
		})
		return
	}

	c.JSON(200, gin.H{
		"status":  "success",
		"message": "Certificate renewed successfully",
		"domain":  domain,
		"output":  string(output),
	})
}

// Delete certificate
func DeleteCertbotCert(c *gin.Context) {
	domain := c.Query("domain")
	if domain == "" {
		c.JSON(400, gin.H{"status": "error", "error": "domain parameter required"})
		return
	}

	args := []string{"delete", "--cert-name", domain, "--non-interactive"}

	cmd := exec.Command("certbot", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		c.JSON(500, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Deletion failed: %v\nOutput: %s", err, string(output)),
		})
		return
	}

	c.JSON(200, gin.H{
		"status":  "success",
		"message": "Certificate deleted successfully",
		"domain":  domain,
	})
}

// List certificates
func ListCertbotCerts(c *gin.Context) {
	cmd := exec.Command("certbot", "certificates")
	output, err := cmd.CombinedOutput()

	if err != nil {
		c.JSON(500, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Failed to list certificates: %v", err),
		})
		return
	}

	// Parse and structure the output
	certs := parseCertbotOutput(string(output))

	c.JSON(200, gin.H{
		"status":       "success",
		"certificates": certs,
	})
}

// Helper function to get certificate expiry
func getCertExpiry(certPath string) string {
	cmd := exec.Command("openssl", "x509", "-in", certPath, "-noout", "-enddate")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	// Parse output like "notAfter=Feb 20 12:34:56 2025 GMT"
	parts := strings.Split(string(output), "=")
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1])
	}
	return "unknown"
}

// Helper function to parse certbot certificates output
func parseCertbotOutput(output string) []map[string]interface{} {
	var certs []map[string]interface{}
	lines := strings.Split(output, "\n")

	var currentCert map[string]interface{}

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "Certificate Name:") {
			if currentCert != nil {
				certs = append(certs, currentCert)
			}
			currentCert = make(map[string]interface{})
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				currentCert["name"] = strings.TrimSpace(parts[1])
			}
		} else if strings.Contains(line, "Domains:") && currentCert != nil {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				domains := strings.Fields(parts[1])
				currentCert["domains"] = domains
			}
		} else if strings.Contains(line, "Expiry Date:") && currentCert != nil {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				currentCert["expiry"] = strings.TrimSpace(parts[1])
			}
		} else if currentCert != nil {
			if strings.Contains(line, "Certificate Path:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					currentCert["cert_path"] = strings.TrimSpace(parts[1])
				}
			} else if strings.Contains(line, "Private Key Path:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					currentCert["key_path"] = strings.TrimSpace(parts[1])
				}
			}
		}
	}

	if currentCert != nil {
		certs = append(certs, currentCert)
	}

	return certs
}

// Verify if certificate is valid and verified for a domain
func verifyCertificateForDomain(domain string) (bool, string) {
	// locate cert path (and implicitly ensure cert exists)
	certPath, _, err := getCertificateAndKeyPaths(domain)
	if err != nil {
		return false, err.Error()
	}

	// make sure the certificate hasn't expired
	cmd := exec.Command("openssl", "x509", "-in", certPath, "-noout", "-checkend", "0")
	if err := cmd.Run(); err != nil {
		return false, fmt.Sprintf("Certificate is expired or invalid for domain: %s", domain)
	}

	// verify the certificate text contains the domain (plain or www)
	cmd = exec.Command("openssl", "x509", "-in", certPath, "-noout", "-text")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, "Failed to verify certificate validity"
	}

	certText := string(output)
	base := strings.TrimPrefix(domain, "www.")
	if !strings.Contains(certText, domain) && !strings.Contains(certText, base) && !strings.Contains(certText, "*"+base) {
		return false, fmt.Sprintf("Certificate is not verified for domain: %s", domain)
	}

	return true, "Certificate verified successfully"
}

// Get certificate path from certbot output
func getCertificatePath(certOutput string, domain string) string {
	lines := strings.Split(certOutput, "\n")
	var inCertBlock bool

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Look for certificate name matching domain
		if strings.Contains(line, "Certificate Name:") && strings.Contains(line, domain) {
			inCertBlock = true
		}

		if inCertBlock && strings.Contains(line, "Certificate Path:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}

		// Reset block if we hit next certificate
		if inCertBlock && i > 0 && strings.Contains(line, "Certificate Name:") && !strings.Contains(lines[i-1], domain) {
			inCertBlock = false
		}
	}

	return ""
}

// Store files in /var/www/html after certificate verification
func StoreFiles(c *gin.Context) {
	domain := c.PostForm("domain")
	if domain == "" {
		c.JSON(400, gin.H{"status": "error", "error": "domain parameter required"})
		return
	}

	// Verify certificate exists and is valid
	isValid, verifyMsg := verifyCertificateForDomain(domain)
	if !isValid {
		c.JSON(400, gin.H{
			"status": "error",
			"error":  verifyMsg,
		})
		return
	}

	// Create directory if it doesn't exist
	storageDir := "/var/www/html"
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		c.JSON(500, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Failed to create directory: %v", err),
		})
		return
	}

	// Get form files
	form, _ := c.MultipartForm()
	files := form.File["files"]

	if len(files) == 0 {
		c.JSON(400, gin.H{
			"status": "error",
			"error":  "No files provided",
		})
		return
	}

	var storedFiles []string
	var failedFiles []string

	// Store each file
	for _, file := range files {
		src, err := file.Open()
		if err != nil {
			failedFiles = append(failedFiles, fmt.Sprintf("%s: %v", file.Filename, err))
			continue
		}
		defer src.Close()

		// Sanitize filename to prevent directory traversal
		filename := filepath.Base(file.Filename)
		filePath := filepath.Join(storageDir, filename)

		dst, err := os.Create(filePath)
		if err != nil {
			failedFiles = append(failedFiles, fmt.Sprintf("%s: %v", file.Filename, err))
			continue
		}
		defer dst.Close()

		// Copy file contents
		if _, err := io.Copy(dst, src); err != nil {
			failedFiles = append(failedFiles, fmt.Sprintf("%s: %v", file.Filename, err))
			continue
		}

		storedFiles = append(storedFiles, filename)
	}

	response := gin.H{
		"status":  "success",
		"domain":  domain,
		"path":    "/var/www/html",
		"stored":  storedFiles,
		"message": "Files stored successfully after certificate verification",
	}

	if len(failedFiles) > 0 {
		response["failed"] = failedFiles
	}

	c.JSON(200, response)
}

// Get certificate and key paths for a domain (handles plain and www variants)
func getCertificateAndKeyPaths(domain string) (string, string, error) {
	cmd := exec.Command("certbot", "certificates")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("failed to query certificates")
	}

	certs := parseCertbotOutput(string(output))

	// helper to match both with and without www prefix
	match := func(d string) bool {
		if d == domain {
			return true
		}
		// ensure consistent www prefix
		base := strings.TrimPrefix(domain, "www.")
		if d == base || d == "www."+base {
			return true
		}
		return false
	}

	for _, cert := range certs {
		if domains, ok := cert["domains"].([]string); ok {
			for _, d := range domains {
				if match(d) {
					certPath, _ := cert["cert_path"].(string)
					keyPath, _ := cert["key_path"].(string)
					if certPath != "" && keyPath != "" {
						return certPath, keyPath, nil
					}
				}
			}
		}
	}

	return "", "", fmt.Errorf("could not find certificate paths for domain: %s", domain)
}

// Generate HTTPS nginx configuration
func generateNginxConfig(domain string, certPath, keyPath string) string {
	nginxTemplate := `
    # HTTPS Nginx configuration for {{.Domain}}
    # Generated automatically - Certificate verified

server {
    listen 80;
    listen [::]:80;
    server_name {{.Domain}} www.{{.Domain}};
    
    # Redirect HTTP to HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name {{.Domain}} www.{{.Domain}};
    
    # SSL Certificate Configuration
    ssl_certificate {{.CertPath}};
    ssl_certificate_key {{.KeyPath}};
    
    # SSL Security Settings
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;
    
    # Root directory for serving files
    root /var/www/html;
    index index.html index.htm;
    
    # Access and error logs
    access_log /var/log/nginx/{{.Domain}}_access.log;
    error_log /var/log/nginx/{{.Domain}}_error.log;
    
    # Location block for static content
    location / {
        try_files $uri $uri/ =404;
    }
    
    # Location block for caching
    location ~* \.(jpg|jpeg|png|gif|ico|css|js|svg|woff|woff2|ttf|eot)$ {
        expires 30d;
        add_header Cache-Control "public, immutable";
    }
}
`

	tmpl, err := template.New("nginx").Parse(nginxTemplate)
	if err != nil {
		return ""
	}

	data := struct {
		Domain   string
		CertPath string
		KeyPath  string
	}{
		Domain:   domain,
		CertPath: certPath,
		KeyPath:  keyPath,
	}

	var buf strings.Builder
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return ""
	}

	return buf.String()
}

// Generate and store nginx configuration after certificate verification
func GenerateAndStoreNginxConfig(c *gin.Context) {
	domain := c.PostForm("domain")
	if domain == "" {
		c.JSON(400, gin.H{"status": "error", "error": "domain parameter required"})
		return
	}

	// Verify certificate exists and is valid
	isValid, verifyMsg := verifyCertificateForDomain(domain)
	if !isValid {
		c.JSON(400, gin.H{
			"status": "error",
			"error":  verifyMsg,
		})
		return
	}

	// Get certificate and key paths
	certPath, keyPath, err := getCertificateAndKeyPaths(domain)
	if err != nil {
		c.JSON(400, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	// Generate nginx configuration
	nginxConfig := generateNginxConfig(domain, certPath, keyPath)
	if nginxConfig == "" {
		c.JSON(500, gin.H{
			"status": "error",
			"error":  "Failed to generate nginx configuration",
		})
		return
	}

	// Create directory if it doesn't exist
	storageDir := "/var/www/html"
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		c.JSON(500, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Failed to create directory: %v", err),
		})
		return
	}

	// Define nginx config file path
	configFilename := domain + ".conf"

	// Write nginx configuration to file
	// Write config to sites-available and enable it in sites-enabled
	sitesAvailable := "/etc/nginx/sites-available"
	sitesEnabled := "/etc/nginx/sites-enabled"

	if err := os.MkdirAll(sitesAvailable, 0755); err != nil {
		c.JSON(500, gin.H{"status": "error", "error": fmt.Sprintf("Failed to create %s: %v", sitesAvailable, err)})
		return
	}

	if err := os.MkdirAll(sitesEnabled, 0755); err != nil {
		c.JSON(500, gin.H{"status": "error", "error": fmt.Sprintf("Failed to create %s: %v", sitesEnabled, err)})
		return
	}

	configPath := filepath.Join(sitesAvailable, configFilename)
	if err := os.WriteFile(configPath, []byte(nginxConfig), 0644); err != nil {
		c.JSON(500, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Failed to write nginx config: %v", err),
		})
		return
	}

	// Create or replace symlink in sites-enabled
	enabledPath := filepath.Join(sitesEnabled, configFilename)
	// remove existing file/symlink if present
	if _, err := os.Lstat(enabledPath); err == nil {
		_ = os.Remove(enabledPath)
	}
	if err := os.Symlink(configPath, enabledPath); err != nil {
		c.JSON(500, gin.H{"status": "error", "error": fmt.Sprintf("Failed to enable site: %v", err)})
		return
	}

	// Also store any additional files provided (in /var/www/html)
	form, _ := c.MultipartForm()
	files := form.File["files"]

	var storedFiles []string
	var failedFiles []string

	storedFiles = append(storedFiles, configFilename)

	// Store each additional file
	for _, file := range files {
		src, err := file.Open()
		if err != nil {
			failedFiles = append(failedFiles, fmt.Sprintf("%s: %v", file.Filename, err))
			continue
		}
		defer src.Close()

		// Sanitize filename to prevent directory traversal
		filename := filepath.Base(file.Filename)
		filePath := filepath.Join(storageDir, filename)

		dst, err := os.Create(filePath)
		if err != nil {
			failedFiles = append(failedFiles, fmt.Sprintf("%s: %v", file.Filename, err))
			continue
		}
		defer dst.Close()

		// Copy file contents
		if _, err := io.Copy(dst, src); err != nil {
			failedFiles = append(failedFiles, fmt.Sprintf("%s: %v", file.Filename, err))
			continue
		}

		storedFiles = append(storedFiles, filename)
	}

	response := gin.H{
		"status":          "success",
		"domain":          domain,
		"path":            "/var/www/html",
		"nginx_conf":      configFilename,
		"sites_available": configPath,
		"sites_enabled":   enabledPath,
		"stored":          storedFiles,
		"cert_path":       certPath,
		"key_path":        keyPath,
		"message":         "Nginx configuration generated and files stored successfully",
	}

	if len(failedFiles) > 0 {
		response["failed"] = failedFiles
	}

	// Test nginx configuration
	nginxTestCmd := exec.Command("nginx", "-t")
	testOut, testErr := nginxTestCmd.CombinedOutput()
	if testErr != nil {
		response["nginx_test"] = string(testOut)
		response["nginx_reload"] = "skipped"
		c.JSON(500, gin.H{"status": "error", "error": fmt.Sprintf("nginx -t failed: %s", string(testOut)), "details": response})
		return
	}

	// Try to reload nginx (prefer systemctl, fallback to nginx -s reload)
	reloadErr := exec.Command("systemctl", "reload", "nginx").Run()
	reloadMethod := "systemctl reload nginx"
	if reloadErr != nil {
		// fallback
		reloadErr = exec.Command("nginx", "-s", "reload").Run()
		reloadMethod = "nginx -s reload"
	}

	if reloadErr != nil {
		response["nginx_test"] = string(testOut)
		response["nginx_reload"] = fmt.Sprintf("failed (%s)", reloadMethod)
	} else {
		response["nginx_test"] = string(testOut)
		response["nginx_reload"] = fmt.Sprintf("success (%s)", reloadMethod)
	}

	c.JSON(200, response)
}
