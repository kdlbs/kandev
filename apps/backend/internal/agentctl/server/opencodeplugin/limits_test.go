package opencodeplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOpenCodePluginRequestLimits(t *testing.T) {
	committer := &fakeCommitter{}
	host, bootstrap, _ := startTestHost(t, committer)
	claim := testClaim()
	setShellClaims(host, []Claim{claim})
	tooMany := make([]ObservationEnvelope, 0, MaxEventsPerRequest+1)
	for index := 0; index <= MaxEventsPerRequest; index++ {
		event := testEvent(claim, "exited", intPtr(0))
		event.EventID = event.EventID + ":" + string(rune('a'+index%26))
		tooMany = append(tooMany, event)
	}
	status, _ := doJSON(t, host, bootstrap, http.MethodPost, "/v1/events", eventBody(bootstrap, tooMany...), "")
	if status != http.StatusBadRequest {
		t.Fatalf("oversized batch status = %d, want 400", status)
	}
	request, err := http.NewRequest(http.MethodPost, bootstrap.Endpoint+"/v1/events", bytes.NewReader(bytes.Repeat([]byte("x"), MaxEventRequestBytes+1)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+bootstrap.Credential)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body status = %d, want 413", response.StatusCode)
	}
}

func TestOpenCodePluginConfigLongPollReturnsOnClaimChange(t *testing.T) {
	committer := &fakeCommitter{}
	host, bootstrap, _ := startTestHost(t, committer)
	done := make(chan int, 1)
	go func() {
		request, err := http.NewRequest(http.MethodGet, bootstrap.Endpoint+"/v1/config?lease="+bootstrap.LeaseID+"&revision=0", nil)
		if err != nil {
			done <- 0
			return
		}
		request.Header.Set("Authorization", "Bearer "+bootstrap.Credential)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			done <- 0
			return
		}
		defer func() { _ = response.Body.Close() }()
		done <- response.StatusCode
	}()
	time.Sleep(50 * time.Millisecond)
	setShellClaims(host, []Claim{testClaim()})
	select {
	case status := <-done:
		if status != http.StatusOK {
			t.Fatalf("long poll status = %d", status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("configuration long poll did not return after a claim change")
	}
}

func TestOpenCodePluginBootstrapIsPrivate(t *testing.T) {
	committer := &fakeCommitter{}
	host, bootstrap, _ := startTestHost(t, committer)
	if bootstrap.Credential == "" || len(bootstrap.Credential) < 16 {
		t.Fatalf("bootstrap credential = %q", bootstrap.Credential)
	}
	info, err := os.Stat(host.BootstrapPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("bootstrap permissions = %o, want 600", info.Mode().Perm())
	}
	if !strings.HasPrefix(bootstrap.Endpoint, "http://127.0.0.1:") {
		t.Fatalf("bootstrap endpoint = %q, want loopback", bootstrap.Endpoint)
	}
}

func TestOpenCodePluginHelloRejectsTrailingContent(t *testing.T) {
	committer := &fakeCommitter{}
	_, bootstrap, _ := startTestHost(t, committer)
	body, _ := json.Marshal(helloBody(bootstrap))
	body = append(body, []byte(`{"second":true}`)...)
	request, err := http.NewRequest(http.MethodPost, bootstrap.Endpoint+"/v1/hello", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+bootstrap.Credential)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusBadRequest {
		raw, _ := io.ReadAll(response.Body)
		t.Fatalf("trailing content status = %d (%s), want 400", response.StatusCode, raw)
	}
}

func TestOpenCodePluginCloseIsIdempotent(t *testing.T) {
	committer := &fakeCommitter{}
	host, _, _ := startTestHost(t, committer)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := host.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := host.Close(ctx); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}
