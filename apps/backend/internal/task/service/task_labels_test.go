package service

import "testing"

func TestEncodeTaskLabelsPreservesCreateEmptySemantics(t *testing.T) {
	if got := EncodeTaskLabels([]string{" ", ""}); got != "" {
		t.Errorf("EncodeTaskLabels(empty) = %q, want empty string", got)
	}
	if got := EncodeTaskLabels([]string{" bug ", "", "bug", "plugin", "plugin"}); got != `["bug","plugin"]` {
		t.Errorf("EncodeTaskLabels() = %q, want normalized labels", got)
	}
}

func TestEncodeTaskLabelsForUpdateClearsWithEmptyArray(t *testing.T) {
	if got := EncodeTaskLabelsForUpdate([]string{" ", ""}); got != `[]` {
		t.Errorf("EncodeTaskLabelsForUpdate(empty) = %q, want []", got)
	}
}
