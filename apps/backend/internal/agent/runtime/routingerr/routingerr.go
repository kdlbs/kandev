// Package routingerr normalizes agent-launch and agent-runtime failures into
// a small set of provider-routing error codes that the office scheduler can
// react to uniformly. Classification combines structured signals (HTTP
// status, exit codes, typed errors) with provider-specific regexes against
// stdout/stderr, plus a phase-based safety net. The package also exposes
// the prober interface used by the scheduler to recover degraded providers.
package routingerr

import (
	"fmt"
	"net/http"
	"time"
)

// Code is the normalized provider-routing error code.
type Code string

const (
	CodeAuthRequired           Code = "auth_required"
	CodeMissingCredentials     Code = "missing_credentials"
	CodeSubscriptionRequired   Code = "subscription_required"
	CodeQuotaLimited           Code = "quota_limited"
	CodeRateLimited            Code = "rate_limited"
	CodeProviderUnavailable    Code = "provider_unavailable"
	CodeModelUnavailable       Code = "model_unavailable"
	CodeProviderNotConfigured  Code = "provider_not_configured"
	CodeUnknownProvider        Code = "unknown_provider_error"
	CodeAgentRuntime           Code = "agent_runtime_error"
	CodeTask                   Code = "task_error"
	CodeRepo                   Code = "repo_error"
	CodePermissionDeniedByUser Code = "permission_denied_by_user"
)

// Confidence reflects how strongly the classifier trusts the matched signal.
type Confidence string

const (
	ConfHigh   Confidence = "high"
	ConfMedium Confidence = "medium"
	ConfLow    Confidence = "low"
)

// Phase is the lifecycle phase in which the failure surfaced.
type Phase string

const (
	PhaseAuthCheck     Phase = "auth_check"
	PhaseProcessStart  Phase = "process_start"
	PhaseSessionInit   Phase = "session_init"
	PhasePromptSend    Phase = "prompt_send"
	PhaseStreaming     Phase = "streaming"
	PhaseToolExecution Phase = "tool_execution"
	PhaseShutdown      Phase = "shutdown"
)

// Error is the classifier's normalized output. It wraps stderr/stdout in
// RawExcerpt after sanitization + truncation.
type Error struct {
	Code            Code
	Confidence      Confidence
	Phase           Phase
	FallbackAllowed bool
	AutoRetryable   bool
	UserAction      bool
	ClassifierRule  string
	ExitCode        *int
	ResetHint       *time.Time
	RawExcerpt      string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.ClassifierRule)
}

// Input is the raw signal bundle adapters pass to Classify.
type Input struct {
	Phase         Phase
	ProviderID    string
	ExitCode      *int
	StructuredErr error
	HTTPStatus    int
	Stderr        string
	Stdout        string
}

const exitCodeBinaryMissing = 127

// Classify normalizes a failure into a routing-aware Error. See package doc.
func Classify(in Input) *Error {
	excerpt := Sanitize(in.Stderr + "\n" + in.Stdout)
	if e := classifyInjection(in, excerpt); e != nil {
		return e
	}
	if e := classifyStructured(in, excerpt); e != nil {
		return applyInvariants(e)
	}
	if e, ok := matchProviderRules(in.ProviderID, in.Stderr+"\n"+in.Stdout); ok {
		e.Phase = in.Phase
		e.ExitCode = in.ExitCode
		e.RawExcerpt = excerpt
		return applyInvariants(e)
	}
	return applyInvariants(classifyByPhase(in, excerpt))
}

func classifyInjection(in Input, excerpt string) *Error {
	inj := getInjection()
	if inj == nil {
		return nil
	}
	code, ok := inj[in.ProviderID]
	if !ok {
		return nil
	}
	e := &Error{
		Code:           code,
		Confidence:     ConfHigh,
		Phase:          in.Phase,
		ClassifierRule: "inject.env",
		ExitCode:       in.ExitCode,
		RawExcerpt:     excerpt,
	}
	return applyInvariants(e)
}

func classifyStructured(in Input, excerpt string) *Error {
	if c := httpStatusToCode(in.HTTPStatus); c != "" {
		return &Error{
			Code:           c,
			Confidence:     ConfHigh,
			Phase:          in.Phase,
			ClassifierRule: fmt.Sprintf("http.%d", in.HTTPStatus),
			ExitCode:       in.ExitCode,
			RawExcerpt:     excerpt,
		}
	}
	if in.ExitCode != nil && *in.ExitCode == exitCodeBinaryMissing {
		return &Error{
			Code:           CodeProviderNotConfigured,
			Confidence:     ConfHigh,
			Phase:          in.Phase,
			ClassifierRule: "exit.127",
			ExitCode:       in.ExitCode,
			RawExcerpt:     excerpt,
		}
	}
	return nil
}

func httpStatusToCode(status int) Code {
	switch status {
	case http.StatusUnauthorized:
		return CodeAuthRequired
	case http.StatusForbidden:
		return CodePermissionDeniedByUser
	case http.StatusPaymentRequired:
		return CodeSubscriptionRequired
	case http.StatusTooManyRequests:
		return CodeRateLimited
	case http.StatusServiceUnavailable:
		return CodeProviderUnavailable
	}
	return ""
}

func classifyByPhase(in Input, excerpt string) *Error {
	switch in.Phase {
	case PhaseAuthCheck, PhaseProcessStart, PhaseSessionInit:
		return &Error{
			Code:           CodeUnknownProvider,
			Confidence:     ConfLow,
			Phase:          in.Phase,
			ClassifierRule: "phase.prestart.unknown",
			ExitCode:       in.ExitCode,
			RawExcerpt:     excerpt,
		}
	}
	return &Error{
		Code:           CodeAgentRuntime,
		Confidence:     ConfLow,
		Phase:          in.Phase,
		ClassifierRule: "phase.poststart.unknown",
		ExitCode:       in.ExitCode,
		RawExcerpt:     excerpt,
	}
}

func applyInvariants(e *Error) *Error {
	switch e.Code {
	case CodeAuthRequired, CodeMissingCredentials, CodeSubscriptionRequired, CodeProviderNotConfigured:
		e.UserAction = true
		e.AutoRetryable = false
		e.FallbackAllowed = true
	case CodeModelUnavailable:
		e.UserAction = true
		e.AutoRetryable = false
		e.FallbackAllowed = true
	case CodeRateLimited, CodeQuotaLimited:
		e.AutoRetryable = true
		e.FallbackAllowed = true
	case CodeProviderUnavailable, CodeUnknownProvider:
		e.AutoRetryable = true
		e.FallbackAllowed = true
	case CodePermissionDeniedByUser, CodeTask, CodeRepo, CodeAgentRuntime:
		e.FallbackAllowed = false
		e.AutoRetryable = false
	}
	if isPostStartPhase(e.Phase) && e.Code == CodeAgentRuntime {
		e.FallbackAllowed = false
	}
	return e
}

func isPostStartPhase(p Phase) bool {
	switch p {
	case PhasePromptSend, PhaseStreaming, PhaseToolExecution, PhaseShutdown:
		return true
	}
	return false
}
