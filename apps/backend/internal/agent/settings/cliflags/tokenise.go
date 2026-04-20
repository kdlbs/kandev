// Package cliflags converts user-authored CLI-flag strings (as stored on
// AgentProfile.CLIFlags) into argv token slices suitable for exec.Cmd.
//
// The input is POSIX-ish: whitespace splits tokens, single and double quotes
// group tokens verbatim, and backslash escapes the next byte. This matches
// what a user would type in a shell without invoking any actual shell
// interpretation, so the result is safe to pass directly as Args.
package cliflags

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/settings/models"
)

// Tokenise returns the argv tokens for a single CLIFlag entry. An empty or
// whitespace-only input yields no tokens. Unterminated quotes are an error
// so the user sees the mistake at save time rather than at task start.
func Tokenise(raw string) ([]string, error) {
	st := &tokeniseState{}
	for i := 0; i < len(raw); i++ {
		i = st.step(raw, i)
	}
	if st.quote != 0 {
		return nil, fmt.Errorf("unterminated %c quote in flag %q", st.quote, raw)
	}
	st.flush()
	return st.tokens, nil
}

// tokeniseState carries the scanner state between step calls.
type tokeniseState struct {
	tokens  []string
	current strings.Builder
	inToken bool
	quote   byte // 0, '\'', or '"'
}

// step consumes one byte at position i and returns the next index to scan.
// Returning an advanced index is how escape handling "skips" the next byte.
func (s *tokeniseState) step(raw string, i int) int {
	ch := raw[i]
	if s.quote != 0 {
		return s.stepInsideQuote(raw, i, ch)
	}
	switch {
	case ch == '\'' || ch == '"':
		s.quote = ch
		s.inToken = true
	case ch == '\\' && i+1 < len(raw):
		s.current.WriteByte(raw[i+1])
		s.inToken = true
		return i + 1
	case ch == ' ' || ch == '\t' || ch == '\n':
		s.flush()
	default:
		s.current.WriteByte(ch)
		s.inToken = true
	}
	return i
}

// stepInsideQuote handles the quoted-string sub-scanner: either the quote
// closes, a double-quote backslash-escape consumes the next byte, or a byte
// is appended verbatim.
func (s *tokeniseState) stepInsideQuote(raw string, i int, ch byte) int {
	if ch == s.quote {
		s.quote = 0
		return i
	}
	if ch == '\\' && s.quote == '"' && i+1 < len(raw) {
		s.current.WriteByte(raw[i+1])
		return i + 1
	}
	s.current.WriteByte(ch)
	s.inToken = true
	return i
}

func (s *tokeniseState) flush() {
	if !s.inToken {
		return
	}
	s.tokens = append(s.tokens, s.current.String())
	s.current.Reset()
	s.inToken = false
}

// Resolve walks a profile's CLIFlags list and returns the concatenated argv
// tokens for every Enabled entry. Disabled entries are skipped silently;
// malformed entries halt the walk with an error that identifies the offender.
func Resolve(flags []models.CLIFlag) ([]string, error) {
	var out []string
	for i, f := range flags {
		if !f.Enabled {
			continue
		}
		tokens, err := Tokenise(f.Flag)
		if err != nil {
			return nil, fmt.Errorf("cli_flags[%d]: %w", i, err)
		}
		out = append(out, tokens...)
	}
	return out, nil
}
