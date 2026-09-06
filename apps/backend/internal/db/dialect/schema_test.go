package dialect

import (
	"strings"
	"testing"
)

func TestRenderSchema(t *testing.T) {
	schema := `CREATE TABLE example (
		id {{identity}} PRIMARY KEY,
		enabled {{bool}} NOT NULL DEFAULT 0,
		created_at {{timestamp}} NOT NULL DEFAULT {{current_time}}
	)`

	sqlite, err := RenderSchema(SQLite3, schema)
	if err != nil {
		t.Fatalf("RenderSchema(sqlite) error = %v", err)
	}
	if !strings.Contains(sqlite, "id INTEGER PRIMARY KEY") ||
		!strings.Contains(sqlite, "enabled INTEGER") ||
		!strings.Contains(sqlite, "created_at DATETIME") ||
		!strings.Contains(sqlite, "DEFAULT CURRENT_TIMESTAMP") {
		t.Errorf("RenderSchema(sqlite) = %q", sqlite)
	}

	postgres, err := RenderSchema(PGX, schema)
	if err != nil {
		t.Fatalf("RenderSchema(postgres) error = %v", err)
	}
	if !strings.Contains(postgres, "id BIGSERIAL PRIMARY KEY") ||
		!strings.Contains(postgres, "enabled BOOLEAN") ||
		!strings.Contains(postgres, "enabled BOOLEAN NOT NULL DEFAULT FALSE") ||
		!strings.Contains(postgres, "created_at TIMESTAMPTZ") {
		t.Errorf("RenderSchema(postgres) = %q", postgres)
	}
}

func TestRenderSchemaRejectsUnknownAndUnexpandedTokens(t *testing.T) {
	for _, test := range []struct {
		name   string
		driver string
		schema string
	}{
		{name: "unknown token", driver: SQLite3, schema: "CREATE TABLE x (v {{uuid}})"},
		{name: "unexpanded token", driver: SQLite3, schema: "CREATE TABLE x (v {{timestamp}} {{other}})"},
		{name: "unknown driver", driver: "mysql", schema: "CREATE TABLE x (v {{timestamp}})"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := RenderSchema(test.driver, test.schema); err == nil {
				t.Fatal("RenderSchema() error = nil, want error")
			}
		})
	}
}

func TestRenderSchemaSupportsBooleanAndCurrentTimeAliases(t *testing.T) {
	for _, token := range []string{schemaTokenBool, schemaTokenBoolean} {
		query, err := RenderSchema(SQLite3, "CREATE TABLE x (enabled "+token+" DEFAULT "+schemaTokenNow+")")
		if err != nil {
			t.Fatalf("RenderSchema(%q) error = %v", token, err)
		}
		if strings.Contains(query, "{{") {
			t.Fatalf("RenderSchema(%q) left token in %q", token, query)
		}
	}
}
