package offlinebudget

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestParseAcceptsStrictBase10IntegerInRange(t *testing.T) {
	cases := map[string]int{"1": 1, "15": 15, "1440": 1440, "007": 7}
	cases[" 30 "] = 30 // surrounding spaces are ignored
	for in, want := range cases {
		got, err := Parse(in)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
}

func TestParseEmptyMeansDefault(t *testing.T) {
	for _, in := range []string{"", "   "} {
		got, err := Parse(in)
		if err != nil || got != DefaultMinutes {
			t.Errorf("Parse(%q) = %d, %v; want default", in, got, err)
		}
	}
}

func TestParseRejectsEverythingElseNamingKeyAndRange(t *testing.T) {
	for _, in := range []string{"0", "-5", "+5", "1.5", "5m", "1441", "99999999999999999999", "1e2", "0x10", "abc"} {
		_, err := Parse(in)
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("Parse(%q) error = %v, want ErrInvalid", in, err)
			continue
		}
		msg := err.Error()
		if !contains(msg, Key) || !contains(msg, "1") || !contains(msg, "1440") {
			t.Errorf("Parse(%q) message %q does not name the key and range", in, msg)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestParseJSONAbsentAndNullMeanDefault(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage("null")} {
		got, err := ParseJSON(raw)
		if err != nil || got != DefaultMinutes {
			t.Errorf("ParseJSON(%s) = %d, %v; want default", raw, got, err)
		}
	}
}

func TestParseJSONAcceptsIntegerInRange(t *testing.T) {
	got, err := ParseJSON(json.RawMessage("20"))
	if err != nil || got != 20 {
		t.Fatalf("ParseJSON(20) = %d, %v", got, err)
	}
}

func TestParseJSONRejectsOtherShapes(t *testing.T) {
	for _, raw := range []string{"0", "-1", "1441", "1.5", "1e2", `"15"`, "true", "[]", "{}", "99999999999999999999"} {
		if _, err := ParseJSON(json.RawMessage(raw)); !errors.Is(err, ErrInvalid) {
			t.Errorf("ParseJSON(%s) error = %v, want ErrInvalid", raw, err)
		}
	}
}

func TestDurationConvertsMinutes(t *testing.T) {
	if Duration(15) != 15*time.Minute {
		t.Fatal("Duration(15) != 15m")
	}
}
