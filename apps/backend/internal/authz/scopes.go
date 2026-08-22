// Package authz owns Kandev's permission vocabulary: the named scopes that
// guard every user-facing action, the fixed roles that grant them, and the
// single resolver that answers "what may this user do in this workspace".
//
// There is deliberately one registry. Authorization used to be a single admin
// bit applied per route; a second mechanism living alongside the first is how
// permissions drift, so every guarded action names a scope from this file and
// nothing else decides.
package authz

import "sort"

// Scope is a named permission. Values are stable wire identifiers: they appear
// in API responses so the UI can label and gate controls, and they are compared
// with ==, so they are never translated.
type Scope string

// Org scopes are granted by the org role and are independent of any workspace.
const (
	ScopeOrgMembersManage  Scope = "org.members.manage"
	ScopeOrgSettingsManage Scope = "org.settings.manage"
	ScopeOrgConfigManage   Scope = "org.config.manage"
)

// Workspace scopes are granted by the workspace role, per workspace.
const (
	ScopeWorkspaceRead    Scope = "workspace.read"
	ScopeWorkspaceManage  Scope = "workspace.manage"
	ScopeTaskWrite        Scope = "task.write"
	ScopeSessionPrompt    Scope = "session.prompt"
	ScopeSessionControl   Scope = "session.control"
	ScopeSessionExec      Scope = "session.exec"
	ScopeRepositoryManage Scope = "repository.manage"
	ScopeSecretManage     Scope = "secret.manage"
	ScopeMemberManage     Scope = "member.manage"
)

// Definition is a registry entry. Description is developer- and UI-facing
// English; the frontend translates by scope ID rather than displaying this.
type Definition struct {
	Scope       Scope  `json:"scope"`
	Kind        Kind   `json:"kind"`
	Description string `json:"description"`
}

// Kind separates scopes granted org-wide from scopes granted per workspace.
type Kind string

const (
	KindOrg       Kind = "org"
	KindWorkspace Kind = "workspace"
)

// registry is the complete set of scopes Kandev enforces. A scope that is not
// here cannot be required, and a scope here with no enforcement call site fails
// the completeness test in registry_test.go.
var registry = []Definition{
	{ScopeOrgMembersManage, KindOrg, "Invite, create, disable, and re-role users"},
	{ScopeOrgSettingsManage, KindOrg, "Change organization-wide settings and defaults"},
	{ScopeOrgConfigManage, KindOrg, "Manage executors, agent profiles, environments, editors, prompts, and notification providers"},

	{ScopeWorkspaceRead, KindWorkspace, "See the workspace, its board, tasks, and transcripts"},
	{ScopeWorkspaceManage, KindWorkspace, "Rename the workspace, change its defaults and visibility, delete it"},
	{ScopeTaskWrite, KindWorkspace, "Create, edit, move, assign, archive, and delete tasks"},
	{ScopeSessionPrompt, KindWorkspace, "Start or resume a session and message an agent"},
	{ScopeSessionControl, KindWorkspace, "Stop or cancel a running agent"},
	{ScopeSessionExec, KindWorkspace, "Open a terminal, run shell commands, write files, and open port previews"},
	{ScopeRepositoryManage, KindWorkspace, "Add or remove workspace repositories"},
	{ScopeSecretManage, KindWorkspace, "Manage workspace secrets and integration credentials"},
	{ScopeMemberManage, KindWorkspace, "Add or remove workspace members and transfer ownership"},
}

// Registry returns every defined scope, ordered by kind then ID.
func Registry() []Definition {
	out := make([]Definition, len(registry))
	copy(out, registry)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Scope < out[j].Scope
	})
	return out
}

// Set is an unordered collection of scopes.
type Set map[Scope]struct{}

// NewSet builds a Set from the given scopes.
func NewSet(scopes ...Scope) Set {
	set := make(Set, len(scopes))
	for _, scope := range scopes {
		set[scope] = struct{}{}
	}
	return set
}

// Has reports whether the set contains the scope. The zero Set holds nothing,
// which is the fail-closed default every caller depends on.
func (s Set) Has(scope Scope) bool {
	_, ok := s[scope]
	return ok
}

// List returns the set's scopes sorted, for stable API responses and tests.
func (s Set) List() []Scope {
	out := make([]Scope, 0, len(s))
	for scope := range s {
		out = append(out, scope)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Strings returns List as plain strings, for JSON payloads.
func (s Set) Strings() []string {
	scopes := s.List()
	out := make([]string, len(scopes))
	for i, scope := range scopes {
		out[i] = string(scope)
	}
	return out
}
