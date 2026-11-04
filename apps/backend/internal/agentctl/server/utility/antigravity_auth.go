package utility

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"github.com/kandev/kandev/internal/agent/antigravityruntime"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	acpclient "github.com/kandev/kandev/internal/agentctl/server/acp"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// probeAuthRequiredError lets the probe preserve initialize data while
// reporting that the operator must sign in.
type probeAuthRequiredError struct {
	response *ProbeResponse
}

type authSessionArgs struct {
	policy  *acpcompat.AuthPolicy
	consent <-chan string
}

func authSessionOptions(args []authSessionArgs) (*acpcompat.AuthPolicy, <-chan string) {
	if len(args) == 0 {
		return nil, nil
	}
	return args[0].policy, args[0].consent
}

func prepareAntigravityUtilityEnv(
	ctx context.Context,
	agentID string,
	cfg *InferenceConfigDTO,
	env []string,
) ([]string, error) {
	if cfg == nil || cfg.AuthPolicy == nil || !acpcompat.IsAntigravity(agentID) {
		return env, nil
	}
	harness := envValue(env, acpcompat.AntigravityHarnessPathEnv)
	if harness == "" {
		bundle, err := antigravityruntime.Resolve(ctx, antigravityruntime.ResolveOptions{})
		if err != nil {
			return nil, fmt.Errorf("resolve Antigravity harness: %w", err)
		}
		harness = bundle.HarnessPath
	}
	return acpcompat.PrepareLocalAntigravityEnv(env, harness)
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func (e *probeAuthRequiredError) Error() string {
	if e == nil || e.response == nil {
		return acpcompat.ErrAuthRequired.Error()
	}
	return e.response.Error
}

func (e *probeAuthRequiredError) Unwrap() error { return acpcompat.ErrAuthRequired }

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

// applyAntigravityAuth explicitly selects the personal subscription method
// before session creation. Background operations cancel on the first consent
// prompt and return auth-required instead of waiting for a browser.
func applyAntigravityAuth(
	ctx context.Context,
	conn *acp.ClientSideConnection,
	agentID string,
	policy *acpcompat.AuthPolicy,
	initResp acp.InitializeResponse,
	consent <-chan string,
) error {
	if policy == nil || !acpcompat.IsAntigravity(agentID) {
		return nil
	}
	if policy.MethodID != acpcompat.AntigravityPersonalAuthMethod {
		return fmt.Errorf("antigravity auth policy selects unsupported method %q", policy.MethodID)
	}
	if !acpcompat.HasAuthMethod(authMethodIDs(initResp.AuthMethods)) {
		return &acpcompat.AuthRequiredError{
			MethodID: policy.MethodID,
			Message:  "Antigravity did not advertise the personal Google sign-in method",
		}
	}
	authCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := conn.Authenticate(authCtx, acp.AuthenticateRequest{
			MethodId: acp.AuthMethodId(policy.MethodID),
		})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			var requestErr *acp.RequestError
			if !errors.As(err, &requestErr) || requestErr.Code != -32000 {
				return describeACPFailure(ctx, "authenticate", err)
			}
			message := "Antigravity personal sign-in is required"
			if detail := streams.SanitizeProviderMessage(err.Error()); detail != "" {
				message += ": " + detail
			}
			return &acpcompat.AuthRequiredError{
				MethodID: policy.MethodID,
				Message:  message,
			}
		}
		return nil
	case rawURL := <-consent:
		cancel()
		waitForAuthExit(done)
		return &acpcompat.AuthRequiredError{
			MethodID:   policy.MethodID,
			Message:    "Antigravity Google sign-in is required",
			ConsentURL: rawURL,
		}
	case <-ctx.Done():
		cancel()
		waitForAuthExit(done)
		return ctx.Err()
	}
}

func waitForAuthExit(done <-chan error) {
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

func isAuthRequiredError(err error) bool {
	return errors.Is(err, acpcompat.ErrAuthRequired)
}
