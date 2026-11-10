package webpush

import (
	"encoding/json"
	"net/url"
	"unicode/utf8"
)

// maxPayloadBytes keeps the encrypted body comfortably below the 4096-byte
// limit push services enforce.
const maxPayloadBytes = 2048

const (
	maxTitleRunes = 80
	maxBodyRunes  = 160
)

func encodePayload(p Payload) ([]byte, error) {
	p.Title = truncateRunes(p.Title, maxTitleRunes)
	p.Body = truncateRunes(p.Body, maxBodyRunes)
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	if len(body) > maxPayloadBytes {
		p.Body = ""
		return json.Marshal(p)
	}
	return body, nil
}

func truncateRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return string(runes[:limit-1]) + "…"
}

// TaskURL is the in-app deep link for a task, optionally focused on a session.
// It is origin-relative so the service worker resolves it against whatever
// origin the user reaches Kandev through.
func TaskURL(taskID, sessionID string) string {
	if taskID == "" {
		return "/"
	}
	link := "/t/" + url.PathEscape(taskID)
	if sessionID != "" {
		link += "?sessionId=" + url.QueryEscape(sessionID)
	}
	return link
}
