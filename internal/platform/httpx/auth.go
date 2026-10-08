package httpx

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const principalKey = "principal"

type Principal struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
}

type TokenVerifier interface {
	VerifyAccess(string) (Principal, error)
}

func Auth(verifier TokenVerifier) gin.HandlerFunc {
	return func(context *gin.Context) {
		parts := strings.Fields(context.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			WriteError(context, &Error{Status: http.StatusUnauthorized, Code: "AUTH_REQUIRED", Message: "请先登录"})
			return
		}
		principal, err := verifier.VerifyAccess(parts[1])
		if err != nil {
			WriteError(context, &Error{Status: http.StatusUnauthorized, Code: "SESSION_INVALID", Message: "登录已失效，请重新登录"})
			return
		}
		context.Set(principalKey, principal)
		context.Next()
	}
}

func PrincipalFrom(context *gin.Context) (Principal, bool) {
	value, exists := context.Get(principalKey)
	if !exists {
		return Principal{}, false
	}
	principal, ok := value.(Principal)
	return principal, ok
}
