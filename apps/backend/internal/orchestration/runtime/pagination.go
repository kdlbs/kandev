package runtime

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
)

type scopedCursor struct {
	Scope string `json:"scope"`
	After string `json:"after"`
}

func cursorScope(parts ...string) string {
	raw, _ := json.Marshal(parts)
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

func encodeScopedCursor(scope, after string) string {
	raw, _ := json.Marshal(scopedCursor{Scope: scope, After: after})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeScopedCursor(raw, scope string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if len(raw) > 2048 {
		return "", fmt.Errorf("invalid cursor")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("invalid cursor")
	}
	var cursor scopedCursor
	if json.Unmarshal(decoded, &cursor) != nil || cursor.Scope != scope || cursor.After == "" || len(cursor.After) > 200 {
		return "", fmt.Errorf("invalid cursor for this scope")
	}
	return cursor.After, nil
}

func boundedPageLimit(c *gin.Context) (int, bool) {
	raw := c.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 {
		c.AbortWithStatus(400)
		return 0, false
	}
	return min(limit, 100), true
}
