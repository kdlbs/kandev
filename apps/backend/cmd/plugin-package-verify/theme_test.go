package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestExpectedKindRejectsBinaryAsTheme(t *testing.T) {
	archive, _ := writeTestPackage(t, "test", "1.0.0")
	var out, errs bytes.Buffer
	code := run([]string{"--archive", archive, "--expected-id", "test", "--expected-version", "1.0.0", "--expected-kind", "theme"}, &out, &errs)
	if code != 1 || !strings.Contains(errs.String(), "kind") {
		t.Fatalf("code %d: %s", code, errs.String())
	}
}
