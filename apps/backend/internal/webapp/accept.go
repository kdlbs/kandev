package webapp

import (
	"strconv"
	"strings"
)

// mediaRange is one comma-separated entry of a parsed Accept header.
type mediaRange struct {
	typ, subtype string
	q            float64
}

// PrefersHTML reports whether header resolves text/html, by the most
// specific matching media range, to a quality strictly greater than the one
// it resolves for application/json (AC-PLATFORM-STARTUP-PROGRESS-003.3). An
// absent or malformed header, or one that does not clear that bar - including
// a bare */* - reports false rather than promoting an unparseable entry.
func PrefersHTML(header string) bool {
	ranges := parseAccept(header)
	return acceptQuality(ranges, "text", "html") > acceptQuality(ranges, "application", "json")
}

// parseAccept parses header into its media ranges, dropping any entry whose
// type/subtype or q parameter cannot be parsed rather than defaulting it to
// an acceptable quality. An explicit q=0 range is kept, not dropped: it is
// how a client excludes a specific type while still accepting a broader
// wildcard, and dropping it would let the wildcard's quality leak through
// for the type the client named as unacceptable.
func parseAccept(header string) []mediaRange {
	if header == "" {
		return nil
	}
	var ranges []mediaRange
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		typePair := strings.SplitN(strings.TrimSpace(fields[0]), "/", 2)
		if len(typePair) != 2 || typePair[0] == "" || typePair[1] == "" {
			continue
		}
		q, ok := acceptQualityParam(fields[1:])
		if !ok {
			continue
		}
		ranges = append(ranges, mediaRange{
			typ:     strings.ToLower(strings.TrimSpace(typePair[0])),
			subtype: strings.ToLower(strings.TrimSpace(typePair[1])),
			q:       q,
		})
	}
	return ranges
}

// acceptQualityParam reads the q parameter from one Accept field's
// ";"-separated parameters, defaulting to 1.0 when none is present. It
// reports ok=false for a q parameter present but unparseable - including a
// value with trailing garbage after the number, or one outside [0, 1] - so
// the caller excludes the range instead of promoting a malformed value to
// accepted. The parameter name is matched case-insensitively per RFC 7231.
func acceptQualityParam(params []string) (float64, bool) {
	q := 1.0
	for _, field := range params {
		key, value, found := strings.Cut(strings.TrimSpace(field), "=")
		if !found || !strings.EqualFold(strings.TrimSpace(key), "q") {
			continue
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || parsed < 0 || parsed > 1 {
			return 0, false
		}
		q = parsed
	}
	return q, true
}

// acceptQuality resolves the quality ranges assigns to typ/subtype: the
// q-value of the highest-specificity matching range (exact type/subtype >
// type/* > */*), or 0 when nothing matches. A tie at the same specificity
// resolves to the highest q among the tied ranges.
func acceptQuality(ranges []mediaRange, typ, subtype string) float64 {
	bestSpecificity := -1
	var bestQ float64
	for _, r := range ranges {
		specificity, ok := matchSpecificity(r, typ, subtype)
		if !ok {
			continue
		}
		if specificity > bestSpecificity || (specificity == bestSpecificity && r.q > bestQ) {
			bestSpecificity = specificity
			bestQ = r.q
		}
	}
	return bestQ
}

func matchSpecificity(r mediaRange, typ, subtype string) (int, bool) {
	switch {
	case r.typ == typ && r.subtype == subtype:
		return 2, true
	case r.typ == typ && r.subtype == "*":
		return 1, true
	case r.typ == "*" && r.subtype == "*":
		return 0, true
	default:
		return 0, false
	}
}
