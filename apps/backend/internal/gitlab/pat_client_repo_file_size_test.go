package gitlab

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestPATClient_GetRepoFileContent_OversizedFileReportsTooLarge(t *testing.T) {
	host, stop := newTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxRepoFileBytes+1)))
	}))
	defer stop()

	_, err := NewPATClient(host, "tok").GetRepoFileContent(context.Background(), "g/p", "big.md", "main")
	if !errors.Is(err, ErrRepoFileTooLarge) {
		t.Fatalf("err = %v, want ErrRepoFileTooLarge", err)
	}
}
