package handlers

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// base directory where site content is stored. This defaults to
// environment variable (the running nginx container uses
// /var/www/html/sites).
var sitesBasePath = "/var/www/html/sites"

func init() {
	if env := os.Getenv("SITE_BASE_PATH"); env != "" {
		sitesBasePath = env
	}
}

// ListSites returns the names of the top-level entries under the sites
// directory. This includes both subdirectories (individual sites) and
// standalone files such as JSON/theme files.
func ListSites(c *gin.Context) {
	entries, err := os.ReadDir(sitesBasePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": err.Error()})
		return
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "sites": names})
}

// ServeSite handles requests for a specific site or file within the
// sites directory. The route should be registered like
// r.GET("/sites/:site/*filepath", handlers.ServeSite)
//
// Examples:
//
//	GET /sites/asd -> serves sites/asd/index.html or directory listing
//	GET /sites/asd/checkout/index.html -> serves that file
//	GET /sites/nigga.json -> serves the JSON file at root
func ServeSite(c *gin.Context) {
	// build the relative path from parameters
	site := c.Param("site")
	relPath := c.Param("filepath") // includes leading '/'

	// combine and clean the path to avoid traversal
	rel := filepath.Clean(site + relPath)
	full := filepath.Join(sitesBasePath, rel)

	// ensure the resolved path is still under sitesBasePath
	absBase, _ := filepath.Abs(sitesBasePath)
	absFull, _ := filepath.Abs(full)
	if !strings.HasPrefix(absFull, absBase) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "invalid path"})
		return
	}

	info, err := os.Stat(absFull)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": err.Error()})
		return
	}

	if info.IsDir() {
		// if directory, try to serve index.html if exists, otherwise list contents
		indexPath := filepath.Join(absFull, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			c.File(indexPath)
			return
		}

		// list directory entries
		entries, err := os.ReadDir(absFull)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "error": err.Error()})
			return
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "path": rel, "entries": names})
		return
	}

	// otherwise serve the file
	c.File(absFull)
}

// GetSiteInfo returns all top‑level resources related to a site name.
// Files are grouped by prefix (html/json/theme/manifest) and directory
// contents are listed if the corresponding folder exists. This is useful
// for clients to discover what assets are available for a given site.
//
// NOTE: the route is registered as `/sites/info/:site` to avoid conflicting
// with the catch-all `/sites/:site/*filepath` path; Gin doesn’t allow static
// segments to share a prefix with a wildcard.
func GetSiteInfo(c *gin.Context) {
	site := c.Param("site")
	base := filepath.Join(sitesBasePath, site)

	info := make(map[string]interface{})

	// check for html file
	htmlPath := base + ".html"
	if _, err := os.Stat(htmlPath); err == nil {
		info["html"] = site + ".html"
	}

	// helper to load json file into interface{}
	loadJSON := func(path string) interface{} {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var v interface{}
		if err := json.Unmarshal(b, &v); err != nil {
			return nil
		}
		return v
	}

	// standard json
	if _, err := os.Stat(base + ".json"); err == nil {
		info["json"] = loadJSON(base + ".json")
	}

	// default theme
	themePath := base + ".default-theme.json"
	if _, err := os.Stat(themePath); err == nil {
		info["default_theme"] = loadJSON(themePath)
	}

	// manifest (suffix may include dash)
	manifestPath := base + "-manifest.json"
	if _, err := os.Stat(manifestPath); err == nil {
		info["manifest"] = loadJSON(manifestPath)
	}

	// directory listing if exists
	if fi, err := os.Stat(base); err == nil && fi.IsDir() {
		entries, err := os.ReadDir(base)
		if err == nil {
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			info["directory"] = names
		}
	}

	if len(info) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "no resources found for site"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "site": site, "info": info})
}

// prepareSiteStorage verifies the certificate for the given domain and
// returns an appropriate storage directory (preferring ./sites/<domain> when
// it exists). An error is returned if the certificate check fails or the
// directory cannot be created.
func prepareSiteStorage(domain string) (string, error) {
	if domain == "" {
		return "", fmt.Errorf("domain parameter required")
	}

	isValid, verifyMsg := verifyCertificateForDomain(domain)
	if !isValid {
		return "", fmt.Errorf("%s", verifyMsg)
	}

	base := strings.TrimPrefix(domain, "www.")
	storageDir := filepath.Join(sitesBasePath, base)
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return "", err
	}
	return storageDir, nil
}

func parseStorageMode(modeRaw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(modeRaw))
	if mode == "" {
		return "upsert", nil
	}
	switch mode {
	case "upsert", "replace_all":
		return mode, nil
	default:
		return "", fmt.Errorf("invalid mode %q, expected upsert or replace_all", modeRaw)
	}
}

func listRelativeFiles(baseDir string) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return result, nil
	}

	err := filepath.WalkDir(baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		result[rel] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// storeUploadedFiles handles the common multipart file handling logic used by
// both StoreFiles and GenerateAndStoreNginxConfig. It writes each provided
// file to disk under storageDir. If an uploaded file is a zip archive it is
// automatically uncompressed into the target directory (the file itself is
// discarded). This allows clients to effectively upload entire folder
// hierarchies in a single form field.
func storeUploadedFiles(c *gin.Context, storageDir string) ([]string, []string, error) {
	form, _ := c.MultipartForm()
	files := form.File["files"]
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("no files provided")
	}

	var stored []string
	var failed []string

	for _, file := range files {
		filename := filepath.Base(file.Filename)
		ext := strings.ToLower(filepath.Ext(filename))

		src, err := file.Open()
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", filename, err))
			continue
		}
		defer src.Close()

		if ext == ".zip" {
			tmp, err := ioutil.TempFile("", "upload-*.zip")
			if err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", filename, err))
				src.Close()
				continue
			}
			io.Copy(tmp, src)
			tmp.Close()
			if err := unzipToDir(tmp.Name(), storageDir); err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", filename, err))
			} else {
				stored = append(stored, filename+" (unzipped)")
			}
			os.Remove(tmp.Name())
			continue
		}

		filePath := filepath.Join(storageDir, filename)
		dst, err := os.Create(filePath)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", filename, err))
			continue
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", filename, err))
			continue
		}

		stored = append(stored, filename)
	}
	return stored, failed, nil
}

func unzipToDir(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		cleanName := filepath.Clean(f.Name)
		if strings.Contains(cleanName, "..") {
			continue
		}

		targetPath := filepath.Join(destDir, cleanName)
		if !strings.HasPrefix(targetPath, filepath.Clean(destDir)+string(os.PathSeparator)) && filepath.Clean(targetPath) != filepath.Clean(destDir) {
			continue
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, f.Mode()); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, copyErr := io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

// Store files in /var/www/html after certificate verification
func StoreFiles(c *gin.Context) {
	domain := c.PostForm("domain")
	storageDir, err := prepareSiteStorage(domain)
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "error": err.Error()})
		return
	}

	mode, err := parseStorageMode(c.DefaultPostForm("mode", "upsert"))
	if err != nil {
		c.JSON(400, gin.H{"status": "error", "error": err.Error()})
		return
	}

	beforeFiles, err := listRelativeFiles(storageDir)
	if err != nil {
		c.JSON(500, gin.H{"status": "error", "error": fmt.Sprintf("failed to inspect existing files: %v", err)})
		return
	}

	if mode == "replace_all" {
		entries, readErr := os.ReadDir(storageDir)
		if readErr != nil && !os.IsNotExist(readErr) {
			c.JSON(500, gin.H{"status": "error", "error": fmt.Sprintf("failed to prepare storage directory: %v", readErr)})
			return
		}
		for _, entry := range entries {
			removePath := filepath.Join(storageDir, entry.Name())
			if rmErr := os.RemoveAll(removePath); rmErr != nil {
				c.JSON(500, gin.H{"status": "error", "error": fmt.Sprintf("failed to clean storage directory: %v", rmErr)})
				return
			}
		}
	}

	storedFiles, failedFiles, fileErr := storeUploadedFiles(c, storageDir)
	if fileErr != nil {
		c.JSON(400, gin.H{"status": "error", "error": fileErr.Error()})
		return
	}

	afterFiles, err := listRelativeFiles(storageDir)
	if err != nil {
		c.JSON(500, gin.H{"status": "error", "error": fmt.Sprintf("failed to inspect stored files: %v", err)})
		return
	}

	created := []string{}
	updated := []string{}
	deleted := []string{}
	unchanged := []string{}

	for p := range afterFiles {
		if _, existed := beforeFiles[p]; existed {
			updated = append(updated, p)
		} else {
			created = append(created, p)
		}
	}
	for p := range beforeFiles {
		if _, stillExists := afterFiles[p]; !stillExists {
			deleted = append(deleted, p)
		}
	}
	for p := range beforeFiles {
		if _, stillExists := afterFiles[p]; stillExists {
			unchanged = append(unchanged, p)
		}
	}

	response := gin.H{
		"status":  "success",
		"domain":  domain,
		"path":    storageDir,
		"stored":  storedFiles,
		"mode":    mode,
		"summary": gin.H{
			"created":   created,
			"updated":   updated,
			"deleted":   deleted,
			"unchanged": unchanged,
		},
		"message": "Files stored successfully after certificate verification",
	}
	if len(failedFiles) > 0 {
		response["failed"] = failedFiles
	}
	c.JSON(200, response)
}
