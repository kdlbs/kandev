package npmresolution

import (
	"reflect"
	"strings"
	"testing"
)

func TestManagedStartupDiagnosticCodes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "transient npm error codes", input: "npm error code ECONNRESET\nnpm error code ECONNREFUSED\nnpm error code ETIMEDOUT\nnpm error code EAI_AGAIN\nnpm error code E502\nnpm error code E503\nnpm error code E504\nnpm error code EBUSY\nnpm error code ENOTEMPTY\nnpm error code EINTEGRITY", want: []string{"ECONNRESET", "ECONNREFUSED", "ETIMEDOUT", "EAI_AGAIN", "E502", "E503", "E504", "EBUSY", "ENOTEMPTY", "EINTEGRITY"}},
		{name: "permanent npm error codes", input: "npm error code EACCES\nnpm error code EPERM\nnpm error code ENOSPC\nnpm error code EROFS\nnpm error code E401\nnpm error code E403\nnpm error code E404\nnpm error code EAUTH\nnpm error code ENEEDAUTH\nnpm error code EBADENGINE", want: []string{"EACCES", "EPERM", "ENOSPC", "EROFS", "E401", "E403", "E404", "EAUTH", "ENEEDAUTH", "EBADENGINE"}},
		{name: "legacy ETARGET", input: "npm ERR! code ETARGET", want: []string{"ETARGET"}},
		{name: "normalizes code", input: "npm ERR! code econnreset", want: []string{"ECONNRESET"}},
		{name: "rejects unknown codes and prose", input: "npm error code EWHATEVER\nregistry returned ECONNRESET after retry", want: nil},
		{name: "rejects non-code npm lines", input: "npm error ECONNRESET\nnpm error code ECONNRESET while connecting", want: nil},
		{name: "rejects oversized input", input: "npm error code ECONNRESET\n" + strings.Repeat("x", maxManagedStartupDiagnosticBytes), want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ManagedStartupDiagnosticCodes(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ManagedStartupDiagnosticCodes() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestManagedStartupCodeClasses(t *testing.T) {
	for _, code := range []string{"ECONNRESET", "ECONNREFUSED", "ETIMEDOUT", "EAI_AGAIN", "E502", "E503", "E504", "EBUSY", "ENOTEMPTY", "EINTEGRITY"} {
		if !IsTransientManagedStartupCode(code) || IsPermanentManagedStartupCode(code) {
			t.Errorf("code %q was not classified as transient only", code)
		}
	}
	for _, code := range []string{"EACCES", "EPERM", "ENOSPC", "EROFS", "E401", "E403", "E404", "EAUTH", "ENEEDAUTH", "EBADENGINE"} {
		if !IsPermanentManagedStartupCode(code) || IsTransientManagedStartupCode(code) {
			t.Errorf("code %q was not classified as permanent only", code)
		}
	}
	for _, code := range []string{"ETARGET", "EWHATEVER", ""} {
		if IsTransientManagedStartupCode(code) || IsPermanentManagedStartupCode(code) {
			t.Errorf("code %q should not be a generic transient or permanent code", code)
		}
	}
}

func TestAnalyzeManagedStartupDiagnosticsDistinguishesUnknownFromEmpty(t *testing.T) {
	for _, tt := range []struct {
		name             string
		input            string
		present          bool
		complete         bool
		unclassifiedCode bool
		codes            []string
	}{
		{name: "empty", complete: true},
		{name: "unclassified canonical code", input: "npm error code EUSAGE", present: true, complete: true, unclassifiedCode: true},
		{name: "incomplete marker", input: "npm error diagnostic incomplete", present: true},
		{name: "oversized known permanent code", input: "npm error code EACCES\n" + strings.Repeat("x", maxManagedStartupDiagnosticBytes), present: true},
		{name: "known permanent code", input: "npm error code EACCES", present: true, complete: true, codes: []string{"EACCES"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := AnalyzeManagedStartupDiagnostics(tt.input)
			if got.Present != tt.present || got.Complete != tt.complete || got.UnclassifiedCode != tt.unclassifiedCode || !reflect.DeepEqual(got.Codes, tt.codes) {
				t.Fatalf("AnalyzeManagedStartupDiagnostics() = %#v, want present=%v complete=%v unclassified=%v codes=%#v", got, tt.present, tt.complete, tt.unclassifiedCode, tt.codes)
			}
		})
	}
}
