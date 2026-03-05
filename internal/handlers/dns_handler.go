package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetDNSInfo(c *gin.Context) {
	data := map[string]string{
		"domain": "google.com",
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   data,
	})
}
