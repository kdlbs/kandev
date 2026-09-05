package handlers

import (
	"reflect"
	"testing"
)

func TestParseCapabilitiesAllowsHostExecutionLease(t *testing.T) {
	t.Parallel()

	got := parseCapabilities("orchestrate, execute, inspect, execute")
	want := []string{"orchestrate", "execute", "inspect"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseCapabilities() = %v, want %v", got, want)
	}
}
