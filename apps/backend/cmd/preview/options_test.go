package main

import "testing"

func TestParseDeployOptionsSupportsSkipDescription(t *testing.T) {
	opts, err := parseDeployOptions([]string{
		"--pr", "3455",
		"--sha", "abc1234",
		"--repo", "kdlbs/kandev",
		"--skip-web-install",
		"--skip-description",
	})
	if err != nil {
		t.Fatalf("parseDeployOptions returned error: %v", err)
	}
	if !opts.skipWebInstall {
		t.Error("expected skip-web-install to be enabled")
	}
	if !opts.skipDescription {
		t.Error("expected skip-description to be enabled")
	}
}

func TestParseCleanupOptionsSupportsSkipDescription(t *testing.T) {
	opts, err := parseCleanupOptions([]string{
		"--pr", "3455",
		"--repo", "kdlbs/kandev",
		"--skip-description",
	})
	if err != nil {
		t.Fatalf("parseCleanupOptions returned error: %v", err)
	}
	if !opts.skipDescription {
		t.Error("expected skip-description to be enabled")
	}
}
