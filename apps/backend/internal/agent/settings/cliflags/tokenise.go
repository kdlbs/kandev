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
// whitespace-only input yields no tokens. Unterminated quotes and trailing
// backslashes are errors so the user sees the mistake at save time rather
// than at task start (where a silent drop would take every other enabled
// flag down with it via cliflags.Resolve).
func Tokenise(raw string) ([]string, error) {
	st := &tokeniseState{}
	for i := 0; i < len(raw); i++ {
		next, err := st.step(raw, i)
		if err != nil {
			return nil, err
		}
		i = next
	}
	if st.quote != 0 {
		return nil, fmt.Errorf("unterminated %c quote", st.quote)
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
func (s *tokeniseState) step(raw string, i int) (int, error) {
	ch := raw[i]
	if s.quote != 0 {
		return s.stepInsideQuote(raw, i, ch)
	}
	switch ch {
	case '\'', '"':
		s.quote = ch
		s.inToken = true
	case '\\':
		if i+1 >= len(raw) {
			return i, fmt.Errorf("trailing backslash")
		}
		s.current.WriteByte(raw[i+1])
		s.inToken = true
		return i + 1, nil
	case ' ', '\t', '\n':
		s.flush()
	default:
		s.current.WriteByte(ch)
		s.inToken = true
	}
	return i, nil
}

// stepInsideQuote handles the quoted-string sub-scanner: either the quote
// closes, a double-quote backslash-escape consumes the next byte, or a byte
// is appended verbatim.
func (s *tokeniseState) stepInsideQuote(raw string, i int, ch byte) (int, error) {
	if ch == s.quote {
		s.quote = 0
		return i, nil
	}
	if ch == '\\' && s.quote == '"' {
		if i+1 >= len(raw) {
			return i, fmt.Errorf("trailing backslash inside %c quote", s.quote)
		}
		s.current.WriteByte(raw[i+1])
		return i + 1, nil
	}
	s.current.WriteByte(ch)
	s.inToken = true
	return i, nil
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
