package runtime

import (
	"fmt"
	"strings"
)

const (
	MemoryKindAgent   = "agent"
	MemoryKindProject = "project"
	MemoryKindTask    = "task"
	MemoryKindSkill   = "skill"
)

// MemoryNamespace is the parsed form of a runtime memory path:
// /workspaces/{workspace}/memory/{kind}s/{id}/{key...}
type MemoryNamespace struct {
	WorkspaceID string
	Kind        string
	ID          string
	Key         string
}

// ParseMemoryNamespace validates and parses an agent-visible memory path.
func ParseMemoryNamespace(path string) (MemoryNamespace, error) {
	cleaned := strings.Trim(path, "/")
	parts := strings.Split(cleaned, "/")
	if len(parts) < 6 || parts[0] != "workspaces" || parts[2] != "memory" {
		return MemoryNamespace{}, fmt.Errorf("invalid memory path")
	}
	kind, ok := parseMemoryKind(parts[3])
	if !ok {
		return MemoryNamespace{}, fmt.Errorf("invalid memory kind: %s", parts[3])
	}
	ns := MemoryNamespace{
		WorkspaceID: parts[1],
		Kind:        kind,
		ID:          parts[4],
		Key:         strings.Join(parts[5:], "/"),
	}
	if ns.WorkspaceID == "" || ns.ID == "" || ns.Key == "" || strings.Contains(ns.Key, "..") {
		return MemoryNamespace{}, fmt.Errorf("invalid memory path")
	}
	return ns, nil
}

// CanAccessMemory checks workspace and capability constraints for a namespace.
func CanAccessMemory(ctx RunContext, ns MemoryNamespace, write bool) bool {
	if ns.WorkspaceID != ctx.WorkspaceID {
		return false
	}
	if write && !ctx.Capabilities.Allows(CapabilityWriteMemory) {
		return false
	}
	if !write && !ctx.Capabilities.Allows(CapabilityReadMemory) {
		return false
	}
	if ns.Kind == MemoryKindAgent && ns.ID != ctx.AgentID {
		return false
	}
	return true
}

func parseMemoryKind(raw string) (string, bool) {
	switch raw {
	case "agents":
		return MemoryKindAgent, true
	case "projects":
		return MemoryKindProject, true
	case "tasks":
		return MemoryKindTask, true
	case "skills":
		return MemoryKindSkill, true
	default:
		return "", false
	}
}
