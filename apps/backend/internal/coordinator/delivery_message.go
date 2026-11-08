package coordinator

import (
	"strconv"
	"strings"
)

// wakeLine is one wake of the turn as the transcript lists it.
type wakeLine struct {
	Kind  WakeKind
	Ref   string
	Title string
	// Detail is bounded provider text for a change wake, rendered as data.
	Detail string
}

const wakeTitleRunes = 80

// buildWakeMessage renders the unattended turn's message: the lines are the
// turn's wakes, oldest first, and their count is the message's N.
func buildWakeMessage(lines []wakeLine) string {
	var b strings.Builder
	b.WriteString("Unattended turn. No person started this turn or is watching it.\n")
	b.WriteString("Events since your last turn (" + strconv.Itoa(len(lines)) + "):\n")
	for _, l := range lines {
		b.WriteString("- " + string(l.Kind) + " on " + l.Ref + " \"" + sanitizeWakeTitle(l.Title) + "\"" + detailSuffix(l.Detail) + "\n")
	}
	b.WriteString("These were current when this turn started and may have changed since; read\n")
	b.WriteString("current state before acting. Propose what should happen. Proposals wait for a\n")
	b.WriteString("manager unless one has allowed automatic creation of tasks.\n")
	return b.String()
}

func sanitizeWakeTitle(title string) string {
	runes := []rune(title)
	if len(runes) > wakeTitleRunes {
		runes = runes[:wakeTitleRunes]
	}
	for i, r := range runes {
		switch r {
		case '\n', '\t', '\r':
			runes[i] = ' '
		case '"':
			runes[i] = '\''
		}
	}
	return string(runes)
}

func detailSuffix(detail string) string {
	if detail == "" {
		return ""
	}
	return " (" + sanitizeWakeTitle(detail) + ")"
}
