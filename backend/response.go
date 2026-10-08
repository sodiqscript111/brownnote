package main

import "github.com/gin-gonic/gin"

func jsonResponse(c *gin.Context, status int, value any) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.JSON(status, value)
}

func fail(c *gin.Context, status int, message string) {
	jsonResponse(c, status, gin.H{"error": message})
}
