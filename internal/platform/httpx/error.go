package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Error struct {
	Status  int
	Code    string
	Message string
	Details map[string]any
}

func (e *Error) Error() string {
	return e.Message
}

func BadRequest(code, message string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: code, Message: message}
}

func WriteError(context *gin.Context, err error) {
	response := gin.H{
		"code":       "INTERNAL_ERROR",
		"message":    "服务暂时不可用",
		"request_id": RequestIDFrom(context),
	}
	status := http.StatusInternalServerError
	if appErr, ok := err.(*Error); ok {
		status = appErr.Status
		response["code"] = appErr.Code
		response["message"] = appErr.Message
		if len(appErr.Details) > 0 {
			response["details"] = appErr.Details
		}
	}
	context.AbortWithStatusJSON(status, response)
}
