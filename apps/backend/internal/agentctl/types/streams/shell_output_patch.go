package streams

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

const MaxShellOutputFieldBytes = 256 * 1024

var ErrShellOutputBaseline = errors.New("shell output baseline is missing or inconsistent")

// ShellOutputState is the bounded normalized baseline for one output epoch.
type ShellOutputState struct {
	Epoch    string          `json:"epoch"`
	Revision uint64          `json:"revision"`
	Output   ShellExecOutput `json:"output"`
}

// ShellOutputEdit replaces a field or drops a prefix and appends decoded UTF-8 bytes.
type ShellOutputEdit struct {
	Operation string `json:"operation"`
	Drop      int    `json:"drop,omitempty"`
	Data      []byte `json:"data,omitempty"`
}

// ShellOutputPatch is the versioned, provider-neutral output delivery body.
type ShellOutputPatch struct {
	// ExitCodePresent distinguishes omission from an explicit unknown exit.
	ExitCodePresent bool             `json:"-"`
	EncodingVersion int              `json:"encoding_version"`
	Epoch           string           `json:"epoch"`
	Revision        uint64           `json:"revision"`
	BaseRevision    uint64           `json:"base_revision"`
	BaseDigest      string           `json:"base_digest,omitempty"`
	ResultDigest    string           `json:"result_digest"`
	Checkpoint      bool             `json:"checkpoint,omitempty"`
	Stdout          *ShellOutputEdit `json:"stdout,omitempty"`
	Stderr          *ShellOutputEdit `json:"stderr,omitempty"`
	ExitCode        *int             `json:"exit_code"`
	Truncated       bool             `json:"truncated,omitempty"`
	StdoutTruncated bool             `json:"stdout_truncated,omitempty"`
	StderrTruncated bool             `json:"stderr_truncated,omitempty"`
}

// EncodeShellOutputPatch returns nil for an unchanged logical output view.
func EncodeShellOutputPatch(base *ShellOutputState, output ShellExecOutput, epoch string, checkpoint bool) (*ShellOutputPatch, error) {
	if epoch == "" {
		return nil, ErrShellOutputBaseline
	}
	if err := validateShellOutput(output); err != nil {
		return nil, err
	}
	output.RawTerminalOutput = ""
	digest := ShellOutputDigest(output)
	patch := &ShellOutputPatch{EncodingVersion: 1, ExitCodePresent: true, Epoch: epoch, Revision: 1,
		ResultDigest: digest, ExitCode: output.ExitCode, Truncated: output.Truncated,
		StdoutTruncated: output.StdoutTruncated, StderrTruncated: output.StderrTruncated}
	var previous ShellExecOutput
	if base != nil && base.Epoch == epoch {
		if base.Revision == 0 || base.Revision == math.MaxUint64 {
			return nil, ErrShellOutputBaseline
		}
		if digest == ShellOutputDigest(base.Output) && !checkpoint {
			return nil, nil
		}
		patch.BaseRevision = base.Revision
		patch.BaseDigest = ShellOutputDigest(base.Output)
		patch.Revision = base.Revision + 1
		previous = base.Output
	} else {
		checkpoint = true
	}
	patch.Checkpoint = checkpoint
	patch.Stdout = encodeShellOutputEdit(previous.Stdout, output.Stdout, checkpoint)
	patch.Stderr = encodeShellOutputEdit(previous.Stderr, output.Stderr, checkpoint)
	return patch, nil
}

// ApplyShellOutputPatch validates the entire result before returning a new state.
func ApplyShellOutputPatch(base *ShellOutputState, patch *ShellOutputPatch) (*ShellOutputState, error) {
	if err := validateShellOutputPatchBase(base, patch); err != nil {
		return nil, err
	}
	var output ShellExecOutput
	if base != nil {
		output = base.Output
	}
	var err error
	output.Stdout, err = applyShellOutputEdit(output.Stdout, patch.Stdout, patch.Checkpoint)
	if err != nil {
		return nil, err
	}
	output.Stderr, err = applyShellOutputEdit(output.Stderr, patch.Stderr, patch.Checkpoint)
	if err != nil {
		return nil, err
	}
	if patch.ExitCodePresent {
		output.ExitCode = patch.ExitCode
	}
	output.Truncated = patch.Truncated
	output.StdoutTruncated = patch.StdoutTruncated
	output.StderrTruncated = patch.StderrTruncated
	output.RawTerminalOutput = ""
	if err := validateShellOutput(output); err != nil {
		return nil, err
	}
	if ShellOutputDigest(output) != patch.ResultDigest {
		return nil, ErrShellOutputBaseline
	}
	return &ShellOutputState{Epoch: patch.Epoch, Revision: patch.Revision, Output: output}, nil
}

// ShellOutputDigest includes presence-independent normalized text and exact result metadata.
func ShellOutputDigest(output ShellExecOutput) string {
	value := struct {
		Stdout          [sha256.Size]byte
		Stderr          [sha256.Size]byte
		ExitCode        *int
		Truncated       bool
		StdoutTruncated bool
		StderrTruncated bool
	}{sha256.Sum256([]byte(output.Stdout)), sha256.Sum256([]byte(output.Stderr)), output.ExitCode, output.Truncated, output.StdoutTruncated, output.StderrTruncated}
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func validateShellOutput(output ShellExecOutput) error {
	for _, text := range []string{output.Stdout, output.Stderr} {
		if len(text) > MaxShellOutputFieldBytes || !utf8.ValidString(text) {
			return fmt.Errorf("invalid bounded UTF-8 shell output")
		}
	}
	return nil
}

func validateShellOutputPatchBase(base *ShellOutputState, patch *ShellOutputPatch) error {
	if patch == nil || patch.EncodingVersion != 1 || patch.Epoch == "" || patch.Revision == 0 {
		return ErrShellOutputBaseline
	}
	if patch.Checkpoint && !patch.ExitCodePresent {
		return ErrShellOutputBaseline
	}
	if patch.Checkpoint && patch.BaseRevision == 0 {
		if patch.Revision != 1 || patch.BaseDigest != "" || (base != nil && base.Epoch == patch.Epoch) {
			return ErrShellOutputBaseline
		}
		return nil
	}
	return validateExistingShellOutputBase(base, patch)
}

func validateExistingShellOutputBase(base *ShellOutputState, patch *ShellOutputPatch) error {
	if base == nil {
		if patch.Checkpoint && patch.BaseRevision > 0 && patch.Revision == patch.BaseRevision+1 {
			return nil
		}
		return ErrShellOutputBaseline
	}
	if base.Epoch != patch.Epoch || base.Revision != patch.BaseRevision ||
		base.Revision == math.MaxUint64 || patch.Revision != base.Revision+1 ||
		ShellOutputDigest(base.Output) != patch.BaseDigest {
		return ErrShellOutputBaseline
	}
	return nil
}

func encodeShellOutputEdit(previous, next string, checkpoint bool) *ShellOutputEdit {
	if checkpoint {
		return &ShellOutputEdit{Operation: "replace", Data: []byte(next)}
	}
	if previous == next {
		return nil
	}
	overlap := shellOutputOverlap(previous, next)
	if overlap == 0 {
		return &ShellOutputEdit{Operation: "replace", Data: []byte(next)}
	}
	return &ShellOutputEdit{Operation: "shift_append", Drop: len(previous) - overlap, Data: []byte(next[overlap:])}
}

func applyShellOutputEdit(previous string, edit *ShellOutputEdit, checkpoint bool) (string, error) {
	if edit == nil {
		if checkpoint {
			return "", ErrShellOutputBaseline
		}
		return previous, nil
	}
	if len(edit.Data) > MaxShellOutputFieldBytes || !utf8.Valid(edit.Data) {
		return "", ErrShellOutputBaseline
	}
	switch edit.Operation {
	case "replace":
		if edit.Drop != 0 {
			return "", ErrShellOutputBaseline
		}
		return string(edit.Data), nil
	case "shift_append":
		if checkpoint || edit.Drop < 0 || edit.Drop > len(previous) {
			return "", ErrShellOutputBaseline
		}
		tail := previous[edit.Drop:]
		if !utf8.ValidString(tail) || len(tail)+len(edit.Data) > MaxShellOutputFieldBytes {
			return "", ErrShellOutputBaseline
		}
		return tail + string(edit.Data), nil
	default:
		return "", ErrShellOutputBaseline
	}
}

// shellOutputOverlap finds a suffix/prefix overlap in linear time, including repeated text.
func shellOutputOverlap(previous, next string) int {
	if len(next) == 0 || len(previous) == 0 {
		return 0
	}
	if overlap, ok := shortShellOutputOverlap(previous, next); ok {
		return overlap
	}
	prefix := shellOutputPrefix(next)
	matched := 0
	for i := range len(previous) {
		for matched > 0 && (matched == len(next) || previous[i] != next[matched]) {
			matched = prefix[matched-1]
		}
		if previous[i] == next[matched] {
			matched++
		}
	}
	for matched > 0 && (!utf8.ValidString(previous[len(previous)-matched:]) || !utf8.ValidString(next[:matched])) {
		matched = prefix[matched-1]
	}
	return matched
}

func shellOutputPrefix(next string) []int {
	prefix := make([]int, len(next))
	for i, j := 1, 0; i < len(next); i++ {
		for j > 0 && next[i] != next[j] {
			j = prefix[j-1]
		}
		if next[i] == next[j] {
			j++
		}
		prefix[i] = j
	}
	return prefix
}

// UnmarshalJSON preserves the distinction between an omitted exit and explicit null.
func (patch *ShellOutputPatch) UnmarshalJSON(data []byte) error {
	type plain ShellOutputPatch
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var presence struct {
		ExitCode json.RawMessage `json:"exit_code"`
	}
	if err := json.Unmarshal(data, &presence); err != nil {
		return err
	}
	value.ExitCodePresent = presence.ExitCode != nil
	*patch = ShellOutputPatch(value)
	return nil
}

// MarshalJSON retains omitted exits so semantic event hashes distinguish them from null.
func (patch ShellOutputPatch) MarshalJSON() ([]byte, error) {
	type plain ShellOutputPatch
	if patch.ExitCodePresent {
		return json.Marshal(plain(patch))
	}
	return json.Marshal(struct {
		plain
		ExitCode json.RawMessage `json:"exit_code,omitempty"`
	}{plain: plain(patch)})
}

func shortShellOutputOverlap(previous, next string) (int, bool) {
	if strings.HasPrefix(next, previous) {
		return len(previous), true
	}
	if strings.HasSuffix(previous, next) {
		return len(next), true
	}
	for drop := max(1, len(previous)-len(next)); drop <= min(64, len(previous)); drop++ {
		if drop < len(previous) && !utf8.RuneStart(previous[drop]) {
			continue
		}
		if strings.HasPrefix(next, previous[drop:]) {
			return len(previous) - drop, true
		}
	}
	return 0, false
}
