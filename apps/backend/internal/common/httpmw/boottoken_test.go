package httpmw

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newBootTokenRouter(token string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/guarded", RequireBootToken(token), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func TestNewBootTokenIsRandomHex(t *testing.T) {
	a, err := NewBootToken()
	if err != nil {
		t.Fatalf("NewBootToken: %v", err)
	}
	b, err := NewBootToken()
	if err != nil {
		t.Fatalf("NewBootToken: %v", err)
	}
	if a == "" || b == "" {
		t.Fatal("NewBootToken returned empty token")
	}
	if a == b {
		t.Fatal("NewBootToken returned identical tokens on successive calls")
	}
	if len(a) != 64 { // 32 bytes hex-encoded
		t.Fatalf("token length = %d, want 64", len(a))
	}
}

func TestRequireBootTokenValidTokenPasses(t *testing.T) {
	r := newBootTokenRouter("secret")
	req := httptest.NewRequest(http.MethodPost, "/guarded", nil)
	req.Header.Set(BootTokenHeader, "secret")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestRequireBootTokenMissingHeaderRejected(t *testing.T) {
	r := newBootTokenRouter("secret")
	req := httptest.NewRequest(http.MethodPost, "/guarded", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRequireBootTokenWrongTokenRejected(t *testing.T) {
	r := newBootTokenRouter("secret")
	req := httptest.NewRequest(http.MethodPost, "/guarded", nil)
	req.Header.Set(BootTokenHeader, "nope")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// TestRequireBootTokenEmptyServerTokenFailsClosed pins the fail-closed
// behavior: a misconfigured (empty) server token must refuse every request
// rather than leave the route open.
func TestRequireBootTokenEmptyServerTokenFailsClosed(t *testing.T) {
	r := newBootTokenRouter("")
	req := httptest.NewRequest(http.MethodPost, "/guarded", nil)
	req.Header.Set(BootTokenHeader, "anything")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (fail-closed)", rec.Code)
	}
}
