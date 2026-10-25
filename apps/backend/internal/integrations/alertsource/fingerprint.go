package alertsource

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"slices"
	"sort"
)

// Fingerprint computes an alert's identity from labels over the given field
// keys, encoding length-prefixed rather than separator-delimited: a label
// value is attacker-influenceable text, and a NUL or comma separator can be
// forged by a crafted value to reproduce a collision. fields is cloned,
// sorted and de-duplicated before use, so a descriptor's declared field
// order (and any accidental repetition) never affects the result — only the
// set of keys does.
//
// An absent key contributes a single 0x00 byte, distinguishing "key declared
// but not present on this alert" from "key present with an empty value"
// (which contributes 0x01 followed by a zero-length value) without
// inventing a marker that could collide with real label content.
//
// Validity of fields is the descriptor's concern (Descriptor.Validate()
// rejects an empty or duplicate-bearing FingerprintFields before a source
// can register); Fingerprint itself is pure and does not re-validate. A nil
// or empty fields slice hashes an empty buffer and returns a fixed value —
// documented, not guarded against here, because guarding against it twice
// would just be a second, unreachable copy of Descriptor.Validate()'s rule.
func Fingerprint(labels map[string]string, fields []string) string {
	keys := slices.Clone(fields)
	sort.Strings(keys)
	keys = slices.Compact(keys)

	var buf []byte
	var tmp [binary.MaxVarintLen64]byte
	for _, k := range keys {
		buf = appendUvarintBytes(buf, tmp[:], k)
		if v, ok := labels[k]; ok {
			buf = append(buf, 0x01)
			buf = appendUvarintBytes(buf, tmp[:], v)
		} else {
			buf = append(buf, 0x00)
		}
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:])
}

// appendUvarintBytes appends uvarint(len(s)) || s to buf.
func appendUvarintBytes(buf, scratch []byte, s string) []byte {
	n := binary.PutUvarint(scratch, uint64(len(s)))
	buf = append(buf, scratch[:n]...)
	buf = append(buf, s...)
	return buf
}

// isFingerprint reports whether s is a syntactically valid Fingerprint
// output: 64 lowercase hex characters. Every store method taking a
// fingerprint applies this before touching the database — the Go half of
// the invariant the fingerprint CHECK constraint (width only, portably)
// cannot express by itself.
func isFingerprint(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isDigit := c >= '0' && c <= '9'
		isLowerHex := c >= 'a' && c <= 'f'
		if !isDigit && !isLowerHex {
			return false
		}
	}
	return true
}
