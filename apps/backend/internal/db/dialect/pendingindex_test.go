package dialect

import "testing"

func TestPendingIDIndexDDL(t *testing.T) {
	tests := []struct {
		name   string
		driver string
		want   string
	}{
		{
			name:   "sqlite",
			driver: SQLite3,
			want: "CREATE INDEX IF NOT EXISTS idx_pending_lookup ON task_session_messages((json_extract(metadata, '$.pending_id')), " +
				"created_at, id) WHERE json_extract(metadata, '$.pending_id') IS NOT NULL",
		},
		{
			name:   "postgres",
			driver: PGX,
			want: "CREATE INDEX IF NOT EXISTS idx_pending_lookup ON task_session_messages((metadata::jsonb->>'pending_id'), " +
				"created_at, id) WHERE metadata::jsonb->>'pending_id' IS NOT NULL",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PendingIDLookupIndexDDL(tt.driver, "idx_pending_lookup", "task_session_messages")
			if got != tt.want {
				t.Fatalf("PendingIDLookupIndexDDL = %q, want %q", got, tt.want)
			}
		})
	}
}
