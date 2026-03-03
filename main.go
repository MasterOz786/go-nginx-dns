package main

import (
	"log"

	"github.com/MasterOz786/go-nginx-dns/internal/handlers"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "OK"})
	})

	// DNS info route was mainly a stub; its functionality has been merged
	// into /health, so we no longer expose a separate endpoint.
	// r.GET("/dns", handlers.GetDNSInfo)
	// Certificate routes
	cert := r.Group("/cert")
	{
		cert.POST("/generate", handlers.GenerateCertbotCert)
		cert.POST("/renew", handlers.RenewCertbotCert)
		cert.DELETE("/delete", handlers.DeleteCertbotCert)
		cert.GET("/list", handlers.ListCertbotCerts)
	}

	// File storage route (requires certificate verification)
	r.POST("/storage/store", handlers.StoreFiles)
	r.POST("/storage/nginx", handlers.GenerateAndStoreNginxConfig)

	// Sites API for reading the directory structure under ./sites
	r.GET("/sites", handlers.ListSites)
	// metadata about grouped files associated with a site name
	// moved outside of the wildcard path to avoid Gin conflict
	// (catch‑all routes cannot coexist with siblings)
	r.GET("/sites/info/:site", handlers.GetSiteInfo)
	// wildcard route for site contents; filepath is optional
	r.GET("/sites/:site/*filepath", handlers.ServeSite)

	log.Println("Server running!")
	r.Run(":8080")
}
