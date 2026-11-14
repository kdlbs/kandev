package changesource

import (
	"os"
	"strings"
	"testing"
)

// Every query must go through Source.sel so PostgreSQL placeholders are
// rebound; a direct sqlx call would pass "?" to PostgreSQL unchanged.
func TestQueriesAreRebound(t *testing.T) {
	src, err := os.ReadFile("source.go")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(src), "s.db.SelectContext("); n != 1 {
		t.Fatalf("found %d direct SelectContext calls, want only the one inside sel", n)
	}
	if n := strings.Count(string(src), "s.db.Rebind("); n != 1 {
		t.Fatalf("found %d Rebind calls, want the one inside sel", n)
	}
}
