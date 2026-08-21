package sqlite

import "fmt"

func primarySessionNotPromoted(sessionID string, requireNonterminal bool) (bool, error) {
	if requireNonterminal {
		return false, nil
	}
	return false, fmt.Errorf("session not found: %s", sessionID)
}
