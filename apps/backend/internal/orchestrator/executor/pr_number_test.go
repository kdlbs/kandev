package executor

import "testing"

// TestPRNumberFromMetadata documents the type-tolerance the helper needs:
// task_repository metadata is round-tripped through JSON, so an integer
// stored on write surfaces as float64 on read. Negative or zero values are
// treated as "no PR" so callers don't accidentally enable the PR refspec
// path with garbage data.
func TestPRNumberFromMetadata(t *testing.T) {
	cases := []struct {
		name string
		md   map[string]interface{}
		want int
	}{
		{name: "nil map", md: nil, want: 0},
		{name: "missing key", md: map[string]interface{}{}, want: 0},
		{name: "float64 (post-json)", md: map[string]interface{}{"pr_number": float64(974)}, want: 974},
		{name: "int (pre-json roundtrip)", md: map[string]interface{}{"pr_number": 42}, want: 42},
		{name: "int64", md: map[string]interface{}{"pr_number": int64(7)}, want: 7},
		{name: "zero is rejected", md: map[string]interface{}{"pr_number": float64(0)}, want: 0},
		{name: "negative is rejected", md: map[string]interface{}{"pr_number": float64(-1)}, want: 0},
		{name: "wrong type ignored", md: map[string]interface{}{"pr_number": "974"}, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := prNumberFromMetadata(tc.md); got != tc.want {
				t.Fatalf("prNumberFromMetadata(%v) = %d, want %d", tc.md, got, tc.want)
			}
		})
	}
}
