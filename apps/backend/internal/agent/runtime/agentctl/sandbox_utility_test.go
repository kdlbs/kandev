//go:build linux

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestSandboxUtilityClientBindsBeforeAcknowledgement(t *testing.T) {
	for _, mode := range []string{"valid", "stale", "refused", "early-response"} {
		t.Run(mode, func(t *testing.T) {
			command := exec.Command("sleep", "30")
			command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			require.NoError(t, command.Start())
			defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
			identity, err := processidentity.Capture(command.Process.Pid)
			require.NoError(t, err)
			launch := &sandbox.Launch{AttemptID: "attempt", Generation: 3, PolicyDigest: "captured-policy"}
			owner := sandbox.ProcessOwner{AttemptID: launch.AttemptID, Generation: launch.Generation, PolicyDigest: launch.PolicyDigest, Supervisor: identity}
			if mode == "stale" {
				owner.Generation++
			}
			var mu sync.Mutex
			bound, acked := false, false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					require.NoError(t, json.NewEncoder(w).Encode(owner))
					return
				}
				mu.Lock()
				defer mu.Unlock()
				if !bound {
					t.Error("acknowledgement arrived before binding")
				}
				acked = true
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
			require.NoError(t, err)
			port, err := strconv.Atoi(portText)
			require.NoError(t, err)
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
			require.NoError(t, err)
			transport := NewClient(host, port, log)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			dispatched := make(chan struct{})
			dispatch := func(ctx context.Context) error {
				close(dispatched)
				if mode == "early-response" {
					return nil
				}
				for {
					mu.Lock()
					done := acked
					mu.Unlock()
					if done {
						return nil
					}
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(time.Millisecond):
					}
				}
			}
			bind := func(_ context.Context, value *sandbox.ProcessOwner) error {
				<-dispatched
				if mode == "refused" {
					return errors.New("ownership rejected")
				}
				require.Equal(t, owner, *value)
				mu.Lock()
				bound = true
				mu.Unlock()
				return nil
			}
			err = transport.WithSandboxOwner(ctx, launch, bind, dispatch)
			if mode == "valid" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			mu.Lock()
			defer mu.Unlock()
			if mode == "stale" || mode == "refused" {
				require.False(t, acked)
			}
		})
	}
}
