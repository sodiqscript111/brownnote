package main

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func handler(c spellChecker, staticDir string) http.Handler {
	router := gin.New()
	router.RedirectTrailingSlash = false
	router.RedirectFixedPath = false
	_ = router.SetTrustedProxies(nil)
	router.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
		fail(c, http.StatusInternalServerError, "internal server error")
		c.Abort()
	}))
	router.POST("/api/check", (&api{checker: c}).serveCheck)
	files := http.FileServer(http.Dir(staticDir))
	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/api/check" {
			c.Header("Allow", "POST")
			fail(c, http.StatusMethodNotAllowed, "use POST")
		} else if path == "/api" || strings.HasPrefix(path, "/api/") {
			fail(c, http.StatusNotFound, "API route not found")
		} else {
			files.ServeHTTP(c.Writer, c.Request)
		}
	})
	return router
}
