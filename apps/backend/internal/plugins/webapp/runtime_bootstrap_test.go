package webapp

import (
	"strings"
	"testing"
)

func TestHostRuntimeBootstrapUsesSameOriginCredentials(t *testing.T) {
	const request = `fetch("./_kandev/v1/context", { credentials: "same-origin", cache: "no-store" })`
	if !strings.Contains(hostRuntimeBootstrap, request) {
		t.Fatalf("host bootstrap does not use same-origin credentials: %s", hostRuntimeBootstrap)
	}
	if strings.Contains(hostRuntimeBootstrap, `credentials: "omit"`) {
		t.Fatal("host bootstrap still omits same-origin credentials")
	}
}
