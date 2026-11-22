// Package reposync holds the provider-neutral pieces shared by the
// repository-backed sync features (Workflow Sync and Prompt Sync): source
// coordinates and their validation, workspace-routed directory listing and
// file reads, sync error classification and sanitizing, and the
// retry/suspension policy applied after a failed attempt.
package reposync
