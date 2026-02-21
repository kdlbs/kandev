package handlers

import "github.com/kandev/kandev/internal/scriptengine"

// ScriptPlaceholder describes a template variable available in prepare/cleanup scripts.
//
// Deprecated: Use scriptengine.PlaceholderInfo directly.
type ScriptPlaceholder = scriptengine.PlaceholderInfo

// scriptPlaceholders is the registry of all available script template placeholders.
var scriptPlaceholders = scriptengine.DefaultPlaceholders
