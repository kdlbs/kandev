package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

var (
	jsonUnmarshal = json.Unmarshal
	jsonMarshal   = json.Marshal
)

func (h *Handlers) fireWebhookTrigger(c *gin.Context) {
	publicID := c.Param("publicId")
	ctx := c.Request.Context()

	trigger, err := h.ctrl.Svc.GetTriggerByPublicID(ctx, publicID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "trigger not found"})
		return
	}
	if !trigger.Enabled {
		c.JSON(http.StatusConflict, gin.H{"error": "trigger is disabled"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot read body"})
		return
	}

	if !verifySignature(trigger.SigningMode, trigger.Secret, c.Request, body) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "signature verification failed"})
		return
	}

	vars := parseWebhookPayload(body)

	routine, err := h.ctrl.Svc.GetRoutine(ctx, trigger.RoutineID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "routine not found"})
		return
	}

	run, err := h.ctrl.Svc.DispatchRoutineRun(ctx, routine, trigger, "webhook", vars)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"run_id": run.ID, "status": run.Status})
}

// verifySignature checks the request against the configured signing mode.
func verifySignature(mode, secret string, r *http.Request, body []byte) bool {
	switch mode {
	case "none", "":
		return true
	case "bearer":
		return verifyBearer(r, secret)
	case "hmac_sha256":
		return verifyHMAC(r, body, secret)
	default:
		return false
	}
}

func verifyBearer(r *http.Request, secret string) bool {
	auth := r.Header.Get("Authorization")
	return strings.TrimPrefix(auth, "Bearer ") == secret
}

func verifyHMAC(r *http.Request, body []byte, secret string) bool {
	sig := r.Header.Get("X-Signature-256")
	if sig == "" {
		return false
	}
	sig = strings.TrimPrefix(sig, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

func parseWebhookPayload(body []byte) map[string]string {
	// Parse JSON body as flat key->string map for variable substitution.
	vars := make(map[string]string)
	if len(body) == 0 {
		return vars
	}
	// Best-effort: try to decode as map[string]interface{} and stringify values.
	var raw map[string]interface{}
	if err := jsonUnmarshal(body, &raw); err != nil {
		return vars
	}
	for k, v := range raw {
		switch val := v.(type) {
		case string:
			vars[k] = val
		default:
			b, _ := jsonMarshal(v)
			vars[k] = string(b)
		}
	}
	return vars
}
