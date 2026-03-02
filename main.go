package main

import (
	"log"
	"github.com/gin-gonic/gin"
	"github.com/MasterOz786/go-nginx-dns/internal/handlers"
)

func main() {
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "OK"})
	})

	r.GET("/dns", handlers.GetDNSInfo)

	log.Println("Server running!")
	r.Run(":8080")
}
