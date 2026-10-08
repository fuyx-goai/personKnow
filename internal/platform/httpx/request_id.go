package httpx

import (
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const RequestIDHeader = "X-Request-ID"

const requestIDKey = "request_id"

var safeRequestID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,79}$`)

func RequestID() gin.HandlerFunc {
	return func(context *gin.Context) {
		requestID := context.GetHeader(RequestIDHeader)
		if !safeRequestID.MatchString(requestID) {
			requestID = uuid.NewString()
		}
		context.Set(requestIDKey, requestID)
		context.Header(RequestIDHeader, requestID)
		context.Next()
	}
}

func RequestIDFrom(context *gin.Context) string {
	requestID, _ := context.Get(requestIDKey)
	value, _ := requestID.(string)
	return value
}
