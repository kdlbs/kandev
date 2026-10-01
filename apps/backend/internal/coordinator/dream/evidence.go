package dream

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	// PromptVersion keys the input hash; it changes with the prompt frame or
	// the answer schema.
	PromptVersion = "dream-v1"
	// MaxEvidenceTurns and MaxEvidenceChars bound the opening message.
	MaxEvidenceTurns = 200
	MaxEvidenceChars = 60000
	// MaxTitleRunes truncates a proposal title.
	MaxTitleRunes = 80

	turnBudgetChars = 40000
)

// Turn is one completed non-dream turn as the opening message projects it.
type Turn struct {
	ID      string
	Trigger string
	Verdict string
	Calls   map[string]int
}

// Decision is one decided proposal as the opening message projects it.
type Decision struct {
	ProposalID   string
	Kind         string
	Decision     string
	EditedFields string
	ReasonCode   string
	TaskResult   string
	CostSubcents *int64
	Title        string
}

// Evidence is the projected evidence of a window.
type Evidence struct {
	// Message is the opening message body.
	Message string
	// TurnIDs are the turns the message includes.
	TurnIDs []string
}

// BuildEvidence projects stored rows into the opening message. Turns arrive
// newest first; the oldest are dropped first when the 200-turn or 60,000
// character caps are reached. It holds no message text, task description,
// child-task conversation or intake text, and wraps what it holds as data.
func BuildEvidence(turns []Turn, decisions []Decision) Evidence {
	var b strings.Builder
	b.WriteString("<data kind=\"coordinator-evidence\">\nTurns (newest first):\n")
	used := utf8.RuneCountInString(b.String())
	var ids []string
	for i, t := range turns {
		if i >= MaxEvidenceTurns {
			break
		}
		line := turnLine(t)
		n := utf8.RuneCountInString(line)
		if used+n > turnBudgetChars {
			break
		}
		b.WriteString(line)
		used += n
		ids = append(ids, t.ID)
	}
	b.WriteString("Decided proposals (newest first):\n")
	used += utf8.RuneCountInString("Decided proposals (newest first):\n")
	for i, d := range decisions {
		if i >= MaxEvidenceTurns {
			break
		}
		line := decisionLine(d)
		n := utf8.RuneCountInString(line)
		if used+n > MaxEvidenceChars-len("</data>\n") {
			break
		}
		b.WriteString(line)
		used += n
	}
	b.WriteString("</data>\n")
	return Evidence{Message: b.String(), TurnIDs: ids}
}

func turnLine(t Turn) string {
	actions := make([]string, 0, len(t.Calls))
	for a := range t.Calls {
		actions = append(actions, a)
	}
	sort.Strings(actions)
	calls := make([]string, 0, len(actions))
	for _, a := range actions {
		calls = append(calls, fmt.Sprintf("%s=%d", a, t.Calls[a]))
	}
	return fmt.Sprintf("- turn %s trigger=%s verdict=%s calls=[%s]\n", t.ID, t.Trigger, t.Verdict, strings.Join(calls, " "))
}

func decisionLine(d Decision) string {
	cost := "unknown"
	if d.CostSubcents != nil {
		cost = fmt.Sprint(*d.CostSubcents)
	}
	return fmt.Sprintf("- proposal %s kind=%s decision=%s edited=%s reason=%s result=%s cost_subcents=%s title=%q\n",
		inert(d.ProposalID), inert(d.Kind), inert(d.Decision), inert(d.EditedFields), inert(d.ReasonCode), inert(d.TaskResult), cost,
		inert(truncateRunes(d.Title, MaxTitleRunes)))
}

// inert keeps free text from closing or opening the evidence envelope.
func inert(s string) string {
	return strings.NewReplacer("<", "&lt;", ">", "&gt;").Replace(s)
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

// InputHash is the SHA-256 of the projected evidence, the prompt version and
// the model that will answer.
func InputHash(e Evidence, model string) string {
	h := sha256.New()
	for _, part := range []string{e.Message, PromptVersion, model} {
		_, _ = fmt.Fprintf(h, "%d:%s|", len(part), part)
	}
	return hex.EncodeToString(h.Sum(nil))
}
