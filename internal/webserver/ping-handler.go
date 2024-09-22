package webserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (w *WebServer) PingHandler(c *gin.Context) {
	c.JSON(http.StatusOK, &gin.H{"message": "pong"})
}
