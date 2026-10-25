package models

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

const (
	// MaxSidebarTaskColorPatchEntries bounds one HTTP or WebSocket mutation.
	MaxSidebarTaskColorPatchEntries = 500
	// MaxSidebarTaskColors bounds the number of stored task decisions, including tombstones.
	MaxSidebarTaskColors = 10_000
	// MaxSidebarTaskColorTaskIDBytes bounds a task ID by its UTF-8 encoded size.
	MaxSidebarTaskColorTaskIDBytes = 128
	// MaxSidebarTaskColorsJSONBytes bounds the encoded stored decision map.
	MaxSidebarTaskColorsJSONBytes = 1 << 20
)

// SidebarTaskColorPatch changes selected personal task-color decisions. A
// nil color is a clear tombstone, and IfMissing is used only by legacy import.
type SidebarTaskColorPatch struct {
	Colors    map[string]*string `json:"colors"`
	IfMissing bool               `json:"if_missing"`
}

// IsValidSidebarTaskColor reports whether value is in the seven-color manual palette.
func IsValidSidebarTaskColor(value string) bool {
	_, ok := sidebarTaskManualColorValues[value]
	return ok
}

// IsValidSidebarTaskColorTaskID reports whether id fits the stored map key contract.
func IsValidSidebarTaskColorTaskID(id string) bool {
	return id != "" && utf8.ValidString(id) && len([]byte(id)) <= MaxSidebarTaskColorTaskIDBytes
}

// ValidateSidebarTaskColorPatch validates a bounded per-task mutation.
func ValidateSidebarTaskColorPatch(patch SidebarTaskColorPatch) error {
	if len(patch.Colors) > MaxSidebarTaskColorPatchEntries {
		return fmt.Errorf("manual colors: max %d patch entries allowed", MaxSidebarTaskColorPatchEntries)
	}
	for taskID, color := range patch.Colors {
		if err := validateSidebarTaskColorTaskID(taskID); err != nil {
			return err
		}
		if color != nil && !IsValidSidebarTaskColor(*color) {
			return fmt.Errorf("manual colors[%s]: unsupported color %q", taskID, *color)
		}
	}
	return nil
}

// ValidateSidebarTaskColors validates the complete stored decision map.
func ValidateSidebarTaskColors(colors map[string]*string) error {
	if len(colors) > MaxSidebarTaskColors {
		return fmt.Errorf("manual colors: max %d stored entries allowed", MaxSidebarTaskColors)
	}
	for taskID, color := range colors {
		if err := validateSidebarTaskColorTaskID(taskID); err != nil {
			return err
		}
		if color != nil && !IsValidSidebarTaskColor(*color) {
			return fmt.Errorf("manual colors[%s]: unsupported color %q", taskID, *color)
		}
	}
	encoded, err := json.Marshal(colors)
	if err != nil {
		return fmt.Errorf("manual colors must be serializable: %w", err)
	}
	if len(encoded) > MaxSidebarTaskColorsJSONBytes {
		return fmt.Errorf("manual colors: max %d encoded bytes allowed", MaxSidebarTaskColorsJSONBytes)
	}
	return nil
}

func validateSidebarTaskColorTaskID(taskID string) error {
	if taskID == "" {
		return fmt.Errorf("manual colors: task id is required")
	}
	if !utf8.ValidString(taskID) || len([]byte(taskID)) > MaxSidebarTaskColorTaskIDBytes {
		return fmt.Errorf("manual colors: task id exceeds %d bytes", MaxSidebarTaskColorTaskIDBytes)
	}
	return nil
}

// CloneSidebarTaskColors returns a deep copy, preserving nil tombstones.
func CloneSidebarTaskColors(colors map[string]*string) map[string]*string {
	clone := make(map[string]*string, len(colors))
	for taskID, color := range colors {
		if color == nil {
			clone[taskID] = nil
			continue
		}
		value := *color
		clone[taskID] = &value
	}
	return clone
}

var sidebarTaskManualColorValues = map[string]struct{}{
	"red":    {},
	"orange": {},
	"yellow": {},
	"green":  {},
	"blue":   {},
	"purple": {},
	"pink":   {},
}
