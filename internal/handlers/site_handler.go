package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// base directory where site content is stored. This is expected to be
// the "sites" directory at the root of the workspace. It can be overridden
// by setting an environment variable if needed in the future.
var sitesBasePath = "./sites"

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
