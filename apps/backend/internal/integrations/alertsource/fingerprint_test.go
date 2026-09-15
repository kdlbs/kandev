package alertsource

import "testing"

func TestFingerprint_StableAcrossFieldOrderAndDuplicates(t *testing.T) {
	labels := map[string]string{"alertname": "DiskFull", "instance": "db-1", "severity": "critical"}

	base := Fingerprint(labels, []string{"alertname", "instance"})
	reordered := Fingerprint(labels, []string{"instance", "alertname"})
	duplicated := Fingerprint(labels, []string{"instance", "alertname", "instance", "alertname"})

	if base != reordered {
		t.Fatalf("field order changed fingerprint: %s vs %s", base, reordered)
	}
	if base != duplicated {
		t.Fatalf("duplicated field changed fingerprint: %s vs %s", base, duplicated)
	}
	if !isFingerprint(base) {
		t.Fatalf("fingerprint is not well-formed: %q", base)
	}
}

func TestFingerprint_DeterministicAcrossRuns(t *testing.T) {
	labels := map[string]string{"a": "1", "b": "2"}
	fields := []string{"a", "b"}
	first := Fingerprint(labels, fields)
	for i := 0; i < 5; i++ {
		if got := Fingerprint(labels, fields); got != first {
			t.Fatalf("run %d: fingerprint changed: %s vs %s", i, got, first)
		}
	}
}

// TestFingerprint_NilOrEmptyFieldsHashesFixedValue is R5-11: Fingerprint is
// pure and does not re-validate FingerprintFields (Descriptor.Validate()
// owns rejecting empty), so a nil or empty fields slice must hash a fixed,
// well-formed value rather than panicking or returning "" — every call site
// in store.go runs isFingerprint on the result before it ever reaches SQL.
func TestFingerprint_NilOrEmptyFieldsHashesFixedValue(t *testing.T) {
	labels := map[string]string{"alertname": "DiskFull"}
	nilFields := Fingerprint(labels, nil)
	emptyFields := Fingerprint(labels, []string{})
	if !isFingerprint(nilFields) {
		t.Fatalf("nil fields: not a well-formed fingerprint: %q", nilFields)
	}
	if nilFields != emptyFields {
		t.Fatalf("nil and empty fields slices must hash identically: %s vs %s", nilFields, emptyFields)
	}
	// Fixed regardless of labels, since no key is ever consulted.
	other := Fingerprint(map[string]string{"different": "labels"}, nil)
	if other != nilFields {
		t.Fatalf("expected the same fixed value regardless of labels, got %s vs %s", other, nilFields)
	}
}

func TestFingerprint_AbsentKeyDiffersFromEmptyValue(t *testing.T) {
	fields := []string{"k"}
	absent := Fingerprint(map[string]string{}, fields)
	empty := Fingerprint(map[string]string{"k": ""}, fields)
	if absent == empty {
		t.Fatal("an absent key and an empty value must not collide")
	}
}

func TestFingerprint_DifferentValuesDifferentHash(t *testing.T) {
	fields := []string{"instance"}
	a := Fingerprint(map[string]string{"instance": "db-1"}, fields)
	b := Fingerprint(map[string]string{"instance": "db-2"}, fields)
	if a == b {
		t.Fatal("different label values produced the same fingerprint")
	}
}

// TestFingerprint_NoForgedBoundary guards D12's stated reason for
// length-prefixing over separator-delimiting: a crafted value must not be
// able to make two different (key, value) pairs hash identically by forging
// a field boundary.
func TestFingerprint_NoForgedBoundary(t *testing.T) {
	fields := []string{"a", "b"}
	honest := Fingerprint(map[string]string{"a": "xy", "b": "z"}, fields)
	forged := Fingerprint(map[string]string{"a": "x", "b": "yz"}, fields)
	if honest == forged {
		t.Fatal("boundary between concatenated fields was forgeable")
	}
}

func TestIsFingerprint(t *testing.T) {
	valid := Fingerprint(map[string]string{"a": "1"}, []string{"a"})
	cases := []struct {
		name string
		s    string
		want bool
	}{
		{"valid", valid, true},
		{"uppercase", "A234567890123456789012345678901234567890123456789012345678901234"[:64], false},
		{"too short", "abcd", false},
		{"non-hex 64 chars", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", false},
		{"empty", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isFingerprint(c.s); got != c.want {
				t.Errorf("isFingerprint(%q) = %v, want %v", c.s, got, c.want)
			}
		})
	}
}
