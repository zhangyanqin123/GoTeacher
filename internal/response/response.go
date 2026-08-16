package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 统一响应结构体
type Response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

// OK 成功响应：HTTP 200 + {code:200, msg:"ok", data}
func OK(c *gin.Context, data any) {
	OKMsg(c, "ok", data)
}

// OKMsg 成功响应并指定 msg（teacher 等前端 mock 约定 msg 为 "success"/"编辑成功" 等）
func OKMsg(c *gin.Context, msg string, data any) {
	c.JSON(http.StatusOK, Response{Code: 200, Msg: msg, Data: data})
}

// Fail 失败响应：约定 businessCode 与 body.code 一致
func Fail(c *gin.Context, httpStatus, businessCode int, msg string) {
	c.JSON(httpStatus, Response{Code: businessCode, Msg: msg})
}
