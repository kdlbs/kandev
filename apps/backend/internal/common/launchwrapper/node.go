package launchwrapper

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
)

// NodeCapability is advertised only while a node wrapper is loaded.
const NodeCapability = "node-launch-wrapper.v1"

var (
	// ErrNodeFile reports a node wrapper file that cannot be trusted.
	ErrNodeFile = errors.New("node launch wrapper file is not usable")
	// ErrNodeRequired reports a launch that needs a node wrapper that is not loaded.
	ErrNodeRequired = errors.New("node launch wrapper is required but not loaded")
)

// Node is the operator-owned wrapper applied outside every other wrapper.
// Its zero value means none is configured.
type Node struct {
	Spec     Spec `json:"-"`
	Required bool `json:"-"`
	// AllocDir is the allocation directory the operator gave agentctl at
	// startup. It resolves AllocDir references and comes from nowhere else.
	AllocDir string `json:"-"`
}

// Active reports whether a launch must go through a node wrapper or be refused.
func (n Node) Active() bool { return n.Required || !n.Spec.IsZero() }

// ValidateNode checks a node envelope. Only this path accepts source "node".
func (s Spec) ValidateNode() error {
	if s.Version != Version || s.Source != SourceNode || !filepath.IsAbs(s.Argv0()) {
		return ErrInvalid
	}
	return ValidateTokens(s.Argv)
}

// Argv0 returns the wrapper executable token, or an empty string.
func (s Spec) Argv0() string {
	if len(s.Argv) == 0 {
		return ""
	}
	return s.Argv[0]
}

// DecodeNode strictly decodes and validates a node envelope.
func DecodeNode(data []byte) (Spec, error) {
	type wire Spec
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return Spec{}, ErrInvalid
	}
	if decoder.More() {
		return Spec{}, ErrInvalid
	}
	spec := Spec(decoded)
	spec.Argv = slices.Clone(spec.Argv)
	if err := spec.ValidateNode(); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

// Prepare composes the final argv: node wrapper, then the profile wrapper when
// present, then the inner command. Without a node wrapper it is the existing
// profile-only preparation, and a required but absent wrapper is refused.
func (n Node) Prepare(profile Spec, inner []string, ctx Context) ([]string, error) {
	if n.Spec.IsZero() {
		if n.Required {
			return nil, ErrNodeRequired
		}
		return Prepare(profile, inner, ctx)
	}
	if err := n.Spec.ValidateNode(); err != nil {
		return nil, err
	}
	if !profile.IsZero() {
		var err error
		if inner, err = Prepare(profile, inner, ctx); err != nil {
			return nil, err
		}
	}
	alloc := ""
	if UsesAllocDir(n.Spec.Argv) {
		var err error
		if alloc, err = resolveAllocDir(n.AllocDir, ctx.WorkDir); err != nil {
			return nil, err
		}
	}
	return prepare(n.Spec, inner, ctx, alloc)
}
