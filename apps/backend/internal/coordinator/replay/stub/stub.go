// Package stub reads the structured answer of a replayed model call. It holds
// no store handle and imports nothing of the coordinator.
package stub

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrUnreadable is wrapped by every parse failure.
var ErrUnreadable = errors.New("replay answer unreadable")

// Proposal kinds a replay answer can hold.
const (
	kindCreateTask = "create_task"
	kindMessage    = "message"
	kindMove       = "move"
	kindResume     = "resume"
)

type element struct {
	Kind         string `json:"kind"`
	TargetTaskID string `json:"target_task_id"`
	WorkflowID   string `json:"workflow_id"`
	Title        string `json:"title"`
}

// Parse reads the answer: exactly one JSON array, bare or inside one markdown
// code fence. It returns the sorted distinct decision keys. Any other text,
// an unknown kind or a missing key field makes the whole answer unreadable.
func Parse(text string) ([]string, error) {
	body, err := arrayText(text)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields()
	var elems []element
	if err := dec.Decode(&elems); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%w: trailing data", ErrUnreadable)
	}
	if _, err := dec.Token(); err == nil {
		return nil, fmt.Errorf("%w: trailing data", ErrUnreadable)
	}
	seen := map[string]bool{}
	keys := []string{}
	for _, e := range elems {
		key, err := e.key()
		if err != nil {
			return nil, err
		}
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func (e element) key() (string, error) {
	switch e.Kind {
	case kindCreateTask:
		if e.WorkflowID == "" || strings.TrimSpace(e.Title) == "" {
			return "", fmt.Errorf("%w: create_task needs workflow_id and title", ErrUnreadable)
		}
		return e.WorkflowID + "|" + strings.ToLower(strings.Join(strings.Fields(e.Title), " ")), nil
	case kindMessage, kindMove, kindResume:
		if e.TargetTaskID == "" {
			return "", fmt.Errorf("%w: %s needs target_task_id", ErrUnreadable, e.Kind)
		}
		return e.Kind + "|" + e.TargetTaskID, nil
	}
	return "", fmt.Errorf("%w: unknown kind %q", ErrUnreadable, e.Kind)
}

// arrayText returns the JSON array text: the whole trimmed text, or the body of
// the single code fence that is the whole trimmed text.
func arrayText(text string) (string, error) {
	t := strings.TrimSpace(text)
	if strings.HasPrefix(t, "```") {
		nl := strings.IndexByte(t, '\n')
		if nl < 0 || !strings.HasSuffix(t, "```") || len(t) < nl+4 {
			return "", fmt.Errorf("%w: malformed fence", ErrUnreadable)
		}
		lang := strings.TrimSpace(t[3:nl])
		if lang != "" && !strings.EqualFold(lang, "json") {
			return "", fmt.Errorf("%w: fence language %q", ErrUnreadable, lang)
		}
		inner := t[nl+1 : len(t)-3]
		if bytes.Contains([]byte(inner), []byte("```")) {
			return "", fmt.Errorf("%w: more than one fence", ErrUnreadable)
		}
		t = strings.TrimSpace(inner)
	}
	if !strings.HasPrefix(t, "[") {
		return "", fmt.Errorf("%w: not an array", ErrUnreadable)
	}
	return t, nil
}
