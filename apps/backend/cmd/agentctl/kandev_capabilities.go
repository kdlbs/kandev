package main

import (
	"flag"
	"net/url"
	"strconv"
)

func runCapabilitiesCmd(args []string) int {
	fs := flag.NewFlagSet("capabilities", flag.ContinueOnError)
	kind := fs.String("kind", "", "Capability kind filter")
	session := fs.String("session", "", "Conversation session attachment evidence")
	after := fs.String("after", "", "Opaque directory cursor")
	limit := fs.Int("limit", 50, "Page size (maximum 100)")
	if fs.Parse(args) != nil {
		return 1
	}
	if *limit < 1 {
		cliError("--limit must be positive")
		return 1
	}
	query := url.Values{"limit": {strconv.Itoa(min(*limit, 100))}}
	for key, value := range map[string]string{"kind": *kind, "session_id": *session, "after": *after} {
		if value != "" {
			query.Set(key, value)
		}
	}
	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}
	body, status, err := client.do("GET", orchestrationAPIPrefix+"/runtime/capabilities?"+query.Encode(), nil)
	return handleResponse(body, status, err)
}
