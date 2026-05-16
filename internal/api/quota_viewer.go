package api

import (
	"embed"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed web/quota
var quotaWebFS embed.FS

func (s *Server) serveAPIKeyQuotaViewer(c *gin.Context) {
	// Serve static files from embedded FS
	path := c.Param("filepath")
	if path == "" || path == "/" {
		path = "index.html"
	}

	// Remove leading slash if present
	path = strings.TrimPrefix(path, "/")

	fileData, err := quotaWebFS.ReadFile("web/quota/" + path)
	if err != nil {
		c.String(http.StatusNotFound, "File not found")
		return
	}

	contentType := "text/plain"
	if strings.HasSuffix(path, ".html") {
		contentType = "text/html; charset=utf-8"
	} else if strings.HasSuffix(path, ".css") {
		contentType = "text/css; charset=utf-8"
	} else if strings.HasSuffix(path, ".js") {
		contentType = "application/javascript; charset=utf-8"
	} else if strings.HasSuffix(path, ".png") {
		contentType = "image/png"
	} else if strings.HasSuffix(path, ".svg") {
		contentType = "image/svg+xml"
	} else if strings.HasSuffix(path, ".ico") {
		contentType = "image/x-icon"
	}

	c.Data(http.StatusOK, contentType, fileData)
}
