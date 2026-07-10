package sqlite

import "testing"

// TestChunkIDs_EmptyReturnsEmptySlice locks in the safety contract: empty or
// nil input must NOT return a single-nil chunk, since
// buildInPlaceholders(nil) is "" and would produce a malformed `IN ()` SQL
// when concatenated by callers like BatchGetSessionsByTaskIDs.
func TestChunkIDs_EmptyReturnsEmptySlice(t *testing.T) {
	for _, tc := range []struct {
		name string
		ids  []string
	}{
		{"nil", nil},
		{"empty", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := chunkIDs(tc.ids, 500)
			if len(got) != 0 {
				t.Errorf("expected no chunks, got %d (a nil/empty chunk would produce SQL syntax error)", len(got))
			}
		})
	}
}

// TestChunkIDs_SinglePass keeps the no-op fast path: when ids fit in one
// chunk the helper returns a single chunk equal to the input slice.
func TestChunkIDs_SinglePass(t *testing.T) {
	ids := []string{"a", "b", "c"}
	got := chunkIDs(ids, 500)
	if len(got) != 1 || len(got[0]) != 3 {
		t.Fatalf("expected one chunk of 3, got %d chunks (sizes %v)", len(got), chunkSizes(got))
	}
}

// TestChunkIDs_SplitsOverThreshold proves the actual chunking branch:
// 7 ids with size=3 must yield chunks of [3, 3, 1] in order.
func TestChunkIDs_SplitsOverThreshold(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e", "f", "g"}
	got := chunkIDs(ids, 3)
	if len(got) != 3 {
		t.Fatalf("expected 3 chunks, got %d (sizes %v)", len(got), chunkSizes(got))
	}
	want := []int{3, 3, 1}
	for i, c := range got {
		if len(c) != want[i] {
			t.Errorf("chunk[%d] size = %d, want %d", i, len(c), want[i])
		}
	}
	// Order across chunks must preserve input order (BatchGet relies on this
	// so per-task sessions land in one chunk together).
	flat := []string{}
	for _, c := range got {
		flat = append(flat, c...)
	}
	for i, id := range ids {
		if flat[i] != id {
			t.Errorf("chunkIDs reordered input at index %d: got %q, want %q", i, flat[i], id)
		}
	}
}

// TestBuildInPlaceholders_NormalCase confirms the placeholder/args shape used
// by BatchGetSessionsByTaskIDs and other IN-clause callers.
func TestBuildInPlaceholders_NormalCase(t *testing.T) {
	ph, args := buildInPlaceholders([]string{"x", "y", "z"})
	if ph != "?,?,?" {
		t.Errorf("placeholders = %q, want %q", ph, "?,?,?")
	}
	if len(args) != 3 || args[0] != "x" || args[1] != "y" || args[2] != "z" {
		t.Errorf("args = %v, want [x y z]", args)
	}
}

// TestBuildInPlaceholders_EmptyReturnsEmpty guards the precondition: empty
// input must yield empty placeholders and nil args so the SQL-syntax-error
// footgun stays contained even if a caller forgets to short-circuit.
func TestBuildInPlaceholders_EmptyReturnsEmpty(t *testing.T) {
	ph, args := buildInPlaceholders(nil)
	if ph != "" || args != nil {
		t.Errorf("expected (\"\", nil), got (%q, %v)", ph, args)
	}
}

func chunkSizes(chunks [][]string) []int {
	out := make([]int, len(chunks))
	for i, c := range chunks {
		out[i] = len(c)
	}
	return out
}
