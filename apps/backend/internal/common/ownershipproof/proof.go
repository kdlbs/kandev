// Package ownershipproof derives the value a control server returns to
// demonstrate it already holds an ownership credential, without disclosing
// it. Both tiers need the identical derivation -- agentctl produces it and
// the backend verifies it -- and agentctl ships as its own binary, so this
// lives in common rather than in either side.
package ownershipproof

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Derive returns the proof for credential over challenge. binding is mixed
// in after a zero separator so a server belonging to a different
// installation cannot answer for this one, and so the challenge and the
// binding cannot run together into an ambiguous input.
func Derive(credential, challenge, binding string) string {
	mac := hmac.New(sha256.New, []byte(credential))
	mac.Write([]byte(challenge))
	mac.Write([]byte{0})
	mac.Write([]byte(binding))
	return hex.EncodeToString(mac.Sum(nil))
}

// Matches reports whether any offered proof is the one credential produces
// for this challenge and binding. A server may hold either of two
// acceptable credentials while a rotation is unconfirmed, so it offers one
// proof per credential and the verifier accepts on any match. Comparison is
// constant-time. An empty credential, an empty challenge, or no offered
// proofs never match: a caller with nothing to verify has proved nothing.
func Matches(credential, challenge, binding string, offered []string) bool {
	if credential == "" || challenge == "" {
		return false
	}
	want := []byte(Derive(credential, challenge, binding))
	matched := false
	for _, candidate := range offered {
		if hmac.Equal([]byte(candidate), want) {
			matched = true
		}
	}
	return matched
}
