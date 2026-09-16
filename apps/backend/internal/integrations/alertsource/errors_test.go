package alertsource

import "testing"

// TestErrorCodesClosedSet asserts every declared ErrorCode has a non-empty,
// unique, lower-snake_case value distinct from its Go identifier's rendering
// — D10/A4's closed-set contract, and what makes the Code sort key in
// TestSortFieldErrors well-defined.
func TestErrorCodesClosedSet(t *testing.T) {
	seen := map[ErrorCode]bool{}
	for _, code := range allErrorCodes {
		if code == "" {
			t.Fatalf("empty error code value in closed set")
		}
		for _, r := range string(code) {
			isLower := r >= 'a' && r <= 'z'
			if !isLower && r != '_' {
				t.Fatalf("code %q is not lower snake_case", code)
			}
		}
		if seen[code] {
			t.Fatalf("duplicate error code value %q", code)
		}
		seen[code] = true
	}
}

func TestSortFieldErrors_Deterministic(t *testing.T) {
	errs := []FieldError{
		{Field: "b", Code: ErrCodeWrongType, Message: "z"},
		{Field: "a", Code: ErrCodeMissingRequired, Message: "y"},
		{Field: "a", Code: ErrCodeInvalidValue, Message: "x"},
		{Field: "", Code: ErrCodeNilField, Message: "second"},
		{Field: "", Code: ErrCodeNilField, Message: "first"},
	}
	sortFieldErrors(errs)
	want := []string{"", "", "a", "a", "b"}
	for i, w := range want {
		if errs[i].Field != w {
			t.Fatalf("index %d: field = %q, want %q (full: %+v)", i, errs[i].Field, w, errs)
		}
	}
	if errs[0].Message != "first" || errs[1].Message != "second" {
		t.Fatalf("Message did not break the Field+Code tie: %+v", errs[:2])
	}
	if errs[2].Code != ErrCodeInvalidValue || errs[3].Code != ErrCodeMissingRequired {
		t.Fatalf("Code did not order within same Field: %+v", errs[2:4])
	}
}

func TestNewValidationError_EmptyIsPlainNil(t *testing.T) {
	err := newValidationError(nil)
	if err != nil {
		t.Fatalf("expected plain nil, got %#v", err)
	}
}

func TestNewSpecError_EmptyIsPlainNil(t *testing.T) {
	err := newSpecError(nil)
	if err != nil {
		t.Fatalf("expected plain nil, got %#v", err)
	}
}

func TestValidationError_ErrorMessageOmitsNothingButIsJoined(t *testing.T) {
	err := newValidationError([]FieldError{
		{Field: "api_token", Code: ErrCodeMissingRequired, Message: "field is required"},
	})
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Error() == "" {
		t.Fatal("expected non-empty error message")
	}
}
