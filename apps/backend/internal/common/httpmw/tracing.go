package httpmw

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/kandev/kandev/internal/agentctl/tracing"
)

// OtelTracing creates a Gin middleware that wraps each request in an OTel span.
// When tracing is disabled (no OTEL_EXPORTER_OTLP_ENDPOINT), this is a no-op.
func OtelTracing(serverName string) gin.HandlerFunc {
	tracer := tracing.Tracer(serverName)

	return func(c *gin.Context) {
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		spanName := fmt.Sprintf("%s %s", c.Request.Method, path)

		ctx, span := tracer.Start(c.Request.Context(), spanName)
		defer span.End()

		c.Request = c.Request.WithContext(ctx)
		c.Next()

		status := c.Writer.Status()
		span.SetAttributes(
			semconv.HTTPRequestMethodKey.String(c.Request.Method),
			semconv.HTTPRouteKey.String(path),
			semconv.HTTPResponseStatusCodeKey.Int(status),
			attribute.Int("http.response.size", c.Writer.Size()),
		)
		if status >= 500 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", status))
		}
	}
}
