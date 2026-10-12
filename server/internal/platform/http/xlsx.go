package http

import (
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

const XLSXContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

func XLSX(c *gin.Context, filename string, data []byte) {
	// A separately hosted frontend needs this header to preserve the server's
	// Chinese filename. Keep any headers already exposed by the CORS middleware.
	exposed := c.Writer.Header().Get("Access-Control-Expose-Headers")
	if !strings.Contains(strings.ToLower(exposed), "content-disposition") {
		if exposed != "" {
			exposed += ", "
		}
		c.Header("Access-Control-Expose-Headers", exposed+"Content-Disposition")
	}
	c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+strings.ReplaceAll(url.QueryEscape(filename), "+", "%20"))
	c.Data(200, XLSXContentType, data)
}
