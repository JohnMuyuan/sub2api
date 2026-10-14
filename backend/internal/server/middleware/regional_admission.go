package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// RegionalAdmission requires every installed regional dependency on each request.
// Apply before model routes, after local authentication; /health is registered
// independently. Passing nil preserves the closed preview behavior.
func RegionalAdmission(ready service.RegionalAdmissionReadiness) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		if ready == nil || ready.Check(ctx) != nil || ctx.Err() != nil {
			c.Header("Cache-Control", "no-store")
			c.Header("Retry-After", "1")
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "regional gateway admission is not ready"})
			return
		}
		// Keep the original request context. The readiness budget must not become
		// the model stream deadline, and no request payload is consumed here.
		c.Next()
	}
}
