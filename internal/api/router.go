package api

import (
	"github.com/gin-gonic/gin"
)

// Router sets up the Gin router and routes.
func Router(chat *ChatHandler) *gin.Engine {
	r := gin.Default()
	r.POST("/v1/chat/completions", chat.HandleChatCompletions)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	return r
}
