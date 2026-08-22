package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeTaskLabels(t *testing.T) {
	cases := []struct {
		name    string
		encoded string
		want    []string
	}{
		{name: "valid array", encoded: `["bug","triage"]`, want: []string{"bug", "triage"}},
		{name: "empty array", encoded: `[]`, want: []string{}},
		{name: "empty string", encoded: "", want: nil},
		{name: "null", encoded: "null", want: nil},
		{name: "non-array object", encoded: `{"a":"b"}`, want: nil},
		{name: "non-array scalar", encoded: `"bug"`, want: nil},
		{name: "malformed JSON", encoded: `[`, want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, DecodeTaskLabels(tc.encoded))
		})
	}
}
