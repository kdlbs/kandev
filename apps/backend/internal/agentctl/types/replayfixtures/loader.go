package replayfixtures

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed fixtures/*.json
var corpusFS embed.FS

// EventTokens is the closed vocabulary for Expect.Events. A fixture whose
// events list (or whose harness-observed sequence) contains any other token
// is a load/assertion error.
var EventTokens = map[string]bool{
	"message_chunk":            true,
	"message_chunk:diagnostic": true,
	"thought_chunk":            true,
	"tool_call":                true,
	"tool_update":              true,
	"error":                    true,
}

// Load reads, parses and validates the embedded fixture corpus. It returns
// every structural error the design calls a load error rather than a silent
// pass, so a single malformed fixture cannot be caught only by whichever test
// happens to load it.
func Load() ([]Fixture, error) {
	entries, err := corpusFS.ReadDir("fixtures")
	if err != nil {
		return nil, fmt.Errorf("read fixtures dir: %w", err)
	}

	fixtures := make([]Fixture, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := corpusFS.ReadFile("fixtures/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		var f Fixture
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&f); err != nil {
			return nil, fmt.Errorf("decode %s: %w", entry.Name(), err)
		}
		f.FileName = entry.Name()
		if err := validateFixture(f); err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		fixtures = append(fixtures, f)
	}

	if err := validateCorpus(fixtures); err != nil {
		return nil, err
	}

	sort.Slice(fixtures, func(i, j int) bool { return fixtures[i].FileName < fixtures[j].FileName })
	return fixtures, nil
}

// MustLoad is Load, panicking on a corpus error. Test packages that import
// this leaf package are expected to call it once at package scope or in
// TestMain, so a corpus defect fails loudly instead of being swallowed by a
// single subtest's error return.
func MustLoad() []Fixture {
	fixtures, err := Load()
	if err != nil {
		panic(err)
	}
	return fixtures
}

func validateFixture(f Fixture) error {
	want, known := classifyingGateways[f.Gateway]
	if !known {
		return fmt.Errorf("unknown gateway %q", f.Gateway)
	}
	if want != f.Classifying {
		return fmt.Errorf("gateway %q classifying=%v disagrees with matrix table (%v)", f.Gateway, f.Classifying, want)
	}

	switch f.Case {
	case CaseMatched, CaseMismatched, CaseLaterOutput, CaseQueued,
		CaseProseMatched, CaseProseTool, CaseProseOutput:
	default:
		return fmt.Errorf("unknown case %q", f.Case)
	}

	if f.AgentID == "" {
		return fmt.Errorf("agentId is required")
	}

	if err := validateCapture(f); err != nil {
		return err
	}
	if err := validateIdentity(f.Identity); err != nil {
		return err
	}
	if err := validateFrames(f.Frames); err != nil {
		return err
	}
	if err := validateExpect(f.Expect); err != nil {
		return err
	}
	return nil
}

func validateCapture(f Fixture) error {
	switch f.Capture {
	case CaptureRecorded:
		if f.Source == "" {
			return fmt.Errorf("recorded fixture requires source")
		}
		if f.CapturedAt == "" {
			return fmt.Errorf("recorded fixture requires capturedAt")
		}
	case CaptureReconstructed:
		if f.Source == "" || strings.HasPrefix(f.Source, "scripts/") {
			return fmt.Errorf("reconstructed fixture requires source citing a published contract, not a capture command")
		}
		if f.CapturedAt != "" {
			return fmt.Errorf("reconstructed fixture must not carry capturedAt")
		}
	default:
		return fmt.Errorf("unknown capture %q", f.Capture)
	}
	return nil
}

func validateIdentity(id Identity) error {
	if id.SessionID == "" {
		return fmt.Errorf("identity.sessionId is required")
	}
	if id.ExecutionID == "" {
		return fmt.Errorf("identity.executionId is required")
	}
	if id.PromptGeneration == 0 {
		return fmt.Errorf("identity.promptGeneration must be non-zero")
	}
	return nil
}

func validateFrames(frames []Frame) error {
	if len(frames) == 0 {
		return fmt.Errorf("frames must be non-empty")
	}
	toolCallIDs := map[string]bool{}
	promptErrorCount := 0
	for i, frame := range frames {
		isLast := i == len(frames)-1
		var err error
		switch frame.Kind {
		case FrameMessageChunk:
			err = validateMessageChunkFrame(i, frame)
		case FrameThoughtChunk:
			err = validateThoughtChunkFrame(i, frame)
		case FrameToolCall:
			err = validateToolCallFrame(i, frame, toolCallIDs)
		case FrameToolUpdate:
			err = validateToolUpdateFrame(i, frame, toolCallIDs)
		case FrameModelSettled:
			err = validateModelSettledFrame(i, frame)
		case FramePromptError:
			promptErrorCount++
			err = validatePromptErrorFrame(i, frame, isLast)
		default:
			err = fmt.Errorf("frame %d: unknown kind %q", i, frame.Kind)
		}
		if err != nil {
			return err
		}
	}
	if promptErrorCount != 1 {
		return fmt.Errorf("frames must carry exactly one prompt_error frame, got %d", promptErrorCount)
	}
	if frames[len(frames)-1].Kind != FramePromptError {
		return fmt.Errorf("the last frame must be prompt_error")
	}
	return nil
}

func validateFrameRoleUnset(i int, role string) error {
	if role != "" {
		return fmt.Errorf("frame %d: role is only valid on message_chunk", i)
	}
	return nil
}

func validateMessageChunkFrame(i int, frame Frame) error {
	if strings.TrimSpace(frame.Text) == "" {
		return fmt.Errorf("frame %d: message_chunk requires non-empty text", i)
	}
	switch frame.Role {
	case "", "assistant", "user":
		return nil
	default:
		return fmt.Errorf("frame %d: unknown role %q", i, frame.Role)
	}
}

func validateThoughtChunkFrame(i int, frame Frame) error {
	if strings.TrimSpace(frame.Text) == "" {
		return fmt.Errorf("frame %d: thought_chunk requires non-empty text", i)
	}
	return validateFrameRoleUnset(i, frame.Role)
}

func validateToolCallFrame(i int, frame Frame, toolCallIDs map[string]bool) error {
	if frame.ToolCallID == "" {
		return fmt.Errorf("frame %d: tool_call requires toolCallId", i)
	}
	if err := validateFrameRoleUnset(i, frame.Role); err != nil {
		return err
	}
	toolCallIDs[frame.ToolCallID] = true
	return nil
}

func validateToolUpdateFrame(i int, frame Frame, toolCallIDs map[string]bool) error {
	if frame.ToolCallID == "" {
		return fmt.Errorf("frame %d: tool_update requires toolCallId", i)
	}
	if !toolCallIDs[frame.ToolCallID] {
		return fmt.Errorf("frame %d: tool_update names toolCallId %q not introduced by an earlier tool_call", i, frame.ToolCallID)
	}
	return validateFrameRoleUnset(i, frame.Role)
}

func validateModelSettledFrame(i int, frame Frame) error {
	if frame.ModelID == "" {
		return fmt.Errorf("frame %d: model_settled requires modelId", i)
	}
	return validateFrameRoleUnset(i, frame.Role)
}

func validatePromptErrorFrame(i int, frame Frame, isLast bool) error {
	if !isLast {
		return fmt.Errorf("frame %d: prompt_error must be the last frame", i)
	}
	if frame.Message == "" {
		return fmt.Errorf("frame %d: prompt_error requires message", i)
	}
	return validateFrameRoleUnset(i, frame.Role)
}

func validateExpect(e Expect) error {
	if len(e.Events) == 0 {
		return fmt.Errorf("expect.events is required")
	}
	for _, token := range e.Events {
		if !EventTokens[token] {
			return fmt.Errorf("expect.events: unknown token %q", token)
		}
	}
	if e.Events[len(e.Events)-1] != "error" {
		return fmt.Errorf("expect.events must end with the error token")
	}
	if e.ProviderError.Source == "" {
		return fmt.Errorf("expect.providerError.source is required")
	}
	if e.ProviderError.ProviderID == "" {
		return fmt.Errorf("expect.providerError.providerId is required")
	}
	if e.DiagnosticCode == "" {
		return fmt.Errorf("expect.diagnosticCode is required")
	}
	return nil
}

// canonicalFrames re-encodes frames through their typed form so object key
// order, indentation and insignificant whitespace cannot be used to dress one
// envelope as two. Raw source bytes are never compared.
func canonicalFrames(frames []Frame) (string, error) {
	b, err := json.Marshal(frames)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func validateCorpus(fixtures []Fixture) error {
	seenPair := map[string]string{}
	seenSession := map[string]string{}
	casesByFrames := map[Case]map[string]string{}

	for _, f := range fixtures {
		pairKey := string(f.Gateway) + "/" + string(f.Case)
		if prev, ok := seenPair[pairKey]; ok {
			return fmt.Errorf("%s and %s both declare (gateway, case) = (%s, %s)", prev, f.FileName, f.Gateway, f.Case)
		}
		seenPair[pairKey] = f.FileName

		if prev, ok := seenSession[f.Identity.SessionID]; ok {
			return fmt.Errorf("%s and %s both use identity.sessionId %q", prev, f.FileName, f.Identity.SessionID)
		}
		seenSession[f.Identity.SessionID] = f.FileName

		canon, err := canonicalFrames(f.Frames)
		if err != nil {
			return fmt.Errorf("%s: canonicalize frames: %w", f.FileName, err)
		}
		byCase := casesByFrames[f.Case]
		if byCase == nil {
			byCase = map[string]string{}
			casesByFrames[f.Case] = byCase
		}
		if prev, ok := byCase[canon]; ok {
			return fmt.Errorf("%s and %s (case %q) carry equivalent frames", prev, f.FileName, f.Case)
		}
		byCase[canon] = f.FileName
	}

	if missing := MissingRequiredCells(fixtures); len(missing) > 0 {
		return fmt.Errorf("missing required fixture cells: %s", strings.Join(missing, ", "))
	}

	return nil
}

// MissingRequiredCells reports every (gateway, case) pair the matrix requires
// that fixtures does not cover. The first four cases are required for all
// four gateways; the three prose- cases are required only for a classifying
// gateway.
func MissingRequiredCells(fixtures []Fixture) []string {
	present := map[string]bool{}
	for _, f := range fixtures {
		present[string(f.Gateway)+"/"+string(f.Case)] = true
	}

	var missing []string
	for gateway, classifying := range classifyingGateways {
		for _, c := range requiredForAllGateways {
			key := string(gateway) + "/" + string(c)
			if !present[key] {
				missing = append(missing, key)
			}
		}
		if classifying {
			for _, c := range requiredForClassifyingGateways {
				key := string(gateway) + "/" + string(c)
				if !present[key] {
					missing = append(missing, key)
				}
			}
		}
	}
	sort.Strings(missing)
	return missing
}
