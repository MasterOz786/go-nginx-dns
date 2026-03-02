package handlers
import (
    "fmt"
    "os/exec"
    "strings"

    "github.com/gin-gonic/gin"
)

type CertRequest struct {
    Domain      string   `json:"domain" binding:"required"`
    Email       string   `json:"email" binding:"required"`
    CertbotMode string   `json:"mode"` // "standalone" or "webroot"
    WebrootPath string   `json:"webroot_path,omitempty"`
    DryRun      bool     `json:"dry_run"`
    Staging     bool     `json:"staging"` // Use staging server for testing
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
    var req CertRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"status": "error", "error": err.Error()})
        return
    }

    // Check if certbot is installed
    if _, err := exec.LookPath("certbot"); err != nil {
        c.JSON(500, gin.H{
            "status": "error",
            "error":  "certbot is not installed. Please install python3-certbot first",
        })
        return
    }

    // Default values
    if req.CertbotMode == "" {
        req.CertbotMode = "standalone"
    }
    if req.Email == "" {
        req.Email = "admin@" + req.Domain
    }

    // Build certbot command
    args := []string{
        "certonly",
        "--non-interactive",
        "--agree-tos",
        "--email", req.Email,
        "-d", req.Domain,
    }

    // Add mode-specific arguments
    switch req.CertbotMode {
    case "standalone":
        args = append(args, "--standalone")
    case "webroot":
        if req.WebrootPath == "" {
            c.JSON(400, gin.H{
                "status": "error",
                "error":  "webroot_path is required for webroot mode",
            })
            return
        }
        args = append(args, "--webroot", "-w", req.WebrootPath)
    default:
        c.JSON(400, gin.H{
            "status": "error",
            "error":  "mode must be either 'standalone' or 'webroot'",
        })
        return
    }

    // Add optional flags
    if req.Staging {
        args = append(args, "--staging")
    }
    if req.DryRun {
        args = append(args, "--dry-run")
    }

    // Create response
    response := CertResponse{
        Status:   "processing",
        Domain:   req.Domain,
        Commands: []string{fmt.Sprintf("certbot %s", strings.Join(args, " "))},
    }

    // Execute certbot
    cmd := exec.Command("certbot", args...)
    output, err := cmd.CombinedOutput()
    
    if err != nil {
        response.Status = "error"
        response.Error = fmt.Sprintf("Certbot failed: %v\nOutput: %s", err, string(output))
        c.JSON(500, response)
        return
    }

    // Success - get certificate paths
    certPath := fmt.Sprintf("/etc/letsencrypt/live/%s/cert.pem", req.Domain)
    keyPath := fmt.Sprintf("/etc/letsencrypt/live/%s/privkey.pem", req.Domain)
    fullchainPath := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", req.Domain)

    response.Status = "success"
    response.Message = "Certificate generated successfully"
    response.CertPath = certPath
    response.KeyPath = keyPath
    response.Fullchain = fullchainPath
    response.ExpiryDate = getCertExpiry(certPath)

    c.JSON(200, response)
}

// Renew certificate
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

