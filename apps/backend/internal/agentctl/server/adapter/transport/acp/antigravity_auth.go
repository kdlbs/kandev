package acp

import (
	"context"
	"fmt"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const antigravityConsentRedaction = "Antigravity Google sign-in URL redacted"

func clientInfoForAgent(agentID string) *acp.Implementation {
	if name, version := acpcompat.ClientInfoForAgent(agentID); name != "" {
		return &acp.Implementation{Name: name, Version: version}
	}
	return &acp.Implementation{Name: "kandev-agentctl", Version: "1.0.0"}
}

func (a *Adapter) applyAntigravityPersonalAuth(ctx context.Context, initResp acp.InitializeResponse) error {
	policy := a.cfg.AuthPolicy
	if policy == nil || !acpcompat.IsAntigravity(a.agentID) {
		return nil
	}
	if policy.MethodID != acpcompat.AntigravityPersonalAuthMethod {
		return fmt.Errorf("antigravity auth policy selects unsupported method %q", policy.MethodID)
	}
	a.setAuthStatus(streams.AuthStatus{
		AttemptID: policy.AttemptID,
		State:     "starting",
	})
	if !acpcompat.HasAuthMethod(authMethodIDs(initResp.AuthMethods)) {
		a.setAuthStatus(streams.AuthStatus{
			AttemptID: policy.AttemptID,
			State:     "failed",
			ErrorCode: "auth_method_missing",
			Error:     "Antigravity did not advertise the personal Google sign-in method",
		})
		return &acpcompat.AuthRequiredError{
			MethodID: policy.MethodID,
			Message:  "Antigravity did not advertise the personal Google sign-in method",
		}
	}
	if policy.AllowInteractive {
		a.setAuthStatus(streams.AuthStatus{
			AttemptID: policy.AttemptID,
			State:     "waiting_for_browser",
		})
		return nil
	}
	return a.authenticateAntigravity(ctx, policy)
}

func (a *Adapter) authenticateAntigravity(ctx context.Context, policy *acpcompat.AuthPolicy) error {
	authCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := a.acpConn.Authenticate(authCtx, acp.AuthenticateRequest{
			MethodId: acp.AuthMethodId(policy.MethodID),
		})
		done <- err
	}()
	consent := a.consentChannel()
	if policy.AllowInteractive {
		consent = nil
	}
	select {
	case err := <-done:
		if err != nil {
			message := "Antigravity personal sign-in is required"
			if detail := streams.SanitizeProviderMessage(err.Error()); detail != "" {
				message += ": " + detail
			}
			a.setAuthStatus(streams.AuthStatus{
				AttemptID: policy.AttemptID,
				State:     "failed",
				ErrorCode: "auth_failed",
				Error:     message,
			})
			return &acpcompat.AuthRequiredError{
				MethodID: policy.MethodID,
				Message:  message,
			}
		}
		a.setAuthStatus(streams.AuthStatus{
			AttemptID: policy.AttemptID,
			State:     "succeeded",
		})
		return nil
	case rawURL := <-consent:
		cancel()
		waitForAntigravityAuthExit(done)
		a.setAuthStatus(streams.AuthStatus{
			AttemptID:        policy.AttemptID,
			State:            "waiting_for_browser",
			AuthorizationURL: rawURL,
		})
		return &acpcompat.AuthRequiredError{
			MethodID:   policy.MethodID,
			Message:    "Antigravity Google sign-in is required",
			ConsentURL: rawURL,
		}
	case <-ctx.Done():
		cancel()
		waitForAntigravityAuthExit(done)
		return ctx.Err()
	}
}

func (a *Adapter) consentChannel() <-chan string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.authConsentCh == nil {
		a.authConsentCh = make(chan string, 1)
	}
	return a.authConsentCh
}

func (a *Adapter) consumeAntigravityStderr(line string) {
	if a.cfg == nil || a.cfg.AuthPolicy == nil {
		return
	}
	raw, ok := acpcompat.ParseConsentURL(line)
	if !ok {
		return
	}
	a.setAuthStatus(streams.AuthStatus{
		AttemptID:        a.cfg.AuthPolicy.AttemptID,
		State:            "waiting_for_browser",
		AuthorizationURL: raw,
	})
	a.mu.Lock()
	ch := a.authConsentCh
	if ch == nil {
		ch = make(chan string, 1)
		a.authConsentCh = ch
	}
	a.mu.Unlock()
	select {
	case ch <- raw:
	default:
	}
}

func (a *Adapter) sanitizeAntigravityStderr(line string) (string, bool) {
	if _, ok := acpcompat.ParseConsentURL(line); ok {
		return antigravityConsentRedaction, true
	}
	if a.cfg != nil && a.cfg.AuthPolicy != nil && strings.Contains(line, "accounts.google.com") {
		return antigravityConsentRedaction, true
	}
	// Provider stderr can contain paths, identifiers, URLs, or credentials.
	// Structured failures arrive through ACP; keep only the redacted marker.
	return "", false
}

func (a *Adapter) setAuthStatus(status streams.AuthStatus) {
	a.mu.Lock()
	a.authStatus = status
	a.mu.Unlock()
}

// AuthStatus returns the transient authentication snapshot.
func (a *Adapter) AuthStatus() streams.AuthStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.authStatus
}

func authMethodIDs(methods []acp.AuthMethod) []string {
	ids := make([]string, 0, len(methods))
	for _, method := range methods {
		id, _, _, _ := acpclient.AuthMethodFields(method)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func waitForAntigravityAuthExit(done <-chan error) {
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}
