package sandbox

// RestartPolicyReview binds an explicit operator choice to an immutable attempt
// and the exact approved selection shown before restart.
type RestartPolicyReview struct {
	AttemptID       string `json:"attempt_id"`
	SelectionDigest string `json:"selection_digest"`
}
