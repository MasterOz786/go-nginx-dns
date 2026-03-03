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

	r.GET("/dns", handlers.GetDNSInfo)
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

	log.Println("Server running!")
	r.Run(":8080")
}
