package httpx

import "github.com/gin-gonic/gin"

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func OK(c *gin.Context, data interface{}) {
	c.JSON(200, Response{Code: 0, Message: "OK", Data: data})
}

func Error(c *gin.Context, status int, code int, message string) {
	c.JSON(status, Response{Code: code, Message: message, Data: nil})
}
