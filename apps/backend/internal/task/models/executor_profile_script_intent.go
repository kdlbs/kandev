package models

// ExecutorProfileScriptIntent distinguishes omitted scripts from explicit clears.
type ExecutorProfileScriptIntent struct {
	PrepareScript *string
	CleanupScript *string
}
