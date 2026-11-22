package types

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/securityutil"
)

const (
	SessionOperationFailureCodeAuthenticationRequired = "authentication_required"
	MaxSessionOperationAuthMethods                    = 8
	MaxSessionOperationAuthMethodBytes                = 4096
	MaxSessionOperationAuthMethodIDBytes              = 128
	MaxSessionOperationAuthMethodNameBytes            = 256
	MaxSessionOperationAuthMethodDescriptionBytes     = 512
	MaxSessionOperationTerminalFieldBytes             = 256
	maxSessionOperationAuthMethodArgs                 = 16
	maxSessionOperationAuthMethodCandidates           = 32
)

// SessionOperationFailure carries the allowlisted cause and bounded login
// methods from one failed ACP session operation.
type SessionOperationFailure struct {
	Code        string                   `json:"code"`
	AuthMethods []streams.AuthMethodInfo `json:"auth_methods,omitempty"`
}

// NormalizeSessionOperationFailure validates protocol evidence and returns a
// safe copy. Unknown causes cannot create session authentication semantics.
func NormalizeSessionOperationFailure(input *SessionOperationFailure) *SessionOperationFailure {
	if input == nil || input.Code != SessionOperationFailureCodeAuthenticationRequired {
		return nil
	}

	out := &SessionOperationFailure{Code: SessionOperationFailureCodeAuthenticationRequired}
	for index, method := range input.AuthMethods {
		if index >= maxSessionOperationAuthMethodCandidates {
			break
		}
		if len(out.AuthMethods) >= MaxSessionOperationAuthMethods {
			break
		}
		if normalized, ok := normalizeSessionOperationAuthMethod(method); ok {
			candidate := append(append([]streams.AuthMethodInfo(nil), out.AuthMethods...), normalized)
			encoded, err := json.Marshal(candidate)
			if err != nil || len(encoded) > MaxSessionOperationAuthMethodBytes {
				continue
			}
			out.AuthMethods = candidate
		}
	}
	return out
}

func normalizeSessionOperationAuthMethod(input streams.AuthMethodInfo) (streams.AuthMethodInfo, bool) {
	id, ok := normalizeAuthText(input.ID, MaxSessionOperationAuthMethodIDBytes)
	if !ok || id == "" || id != input.ID {
		return streams.AuthMethodInfo{}, false
	}

	method := streams.AuthMethodInfo{ID: id}
	if name, ok := normalizeAuthText(input.Name, MaxSessionOperationAuthMethodNameBytes); ok {
		method.Name = name
	}
	if description, ok := normalizeAuthText(input.Description, MaxSessionOperationAuthMethodDescriptionBytes); ok {
		method.Description = description
	}
	method.TerminalAuth = normalizeSessionOperationTerminalAuth(input.TerminalAuth)
	return method, true
}

func normalizeSessionOperationTerminalAuth(input *streams.TerminalAuth) *streams.TerminalAuth {
	if input == nil || len(input.Args) > maxSessionOperationAuthMethodArgs {
		return nil
	}
	command, ok := normalizeTerminalAuthWord(input.Command)
	if !ok {
		return nil
	}
	terminal := &streams.TerminalAuth{Command: command}
	for _, arg := range input.Args {
		safeArg, ok := normalizeTerminalAuthWord(arg)
		if !ok {
			return nil
		}
		terminal.Args = append(terminal.Args, safeArg)
	}
	if label, ok := normalizeAuthText(input.Label, MaxSessionOperationTerminalFieldBytes); ok {
		terminal.Label = label
	}
	return terminal
}

func normalizeTerminalAuthWord(value string) (string, bool) {
	word, ok := normalizeAuthText(value, MaxSessionOperationTerminalFieldBytes)
	if !ok || word == "" || word != securityutil.SanitizeFullUnbounded(value) {
		return "", false
	}
	for _, r := range word {
		if !isSafeTerminalAuthCharacter(r) {
			return "", false
		}
	}
	return word, true
}

func isSafeTerminalAuthCharacter(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	default:
		return strings.ContainsRune("_./:@%+=,-", r)
	}
}

func normalizeAuthText(value string, limit int) (string, bool) {
	if !utf8.ValidString(value) {
		return "", false
	}
	safe := strings.TrimSpace(securityutil.SanitizeFullUnbounded(value))
	if len(safe) > limit {
		return "", false
	}
	for _, r := range safe {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return safe, safe != ""
}
