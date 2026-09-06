package dialect

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	schemaTokenTimestamp   = "{{timestamp}}"
	schemaTokenBool        = "{{bool}}"
	schemaTokenBoolean     = "{{boolean}}"
	schemaTokenIdentity    = "{{identity}}"
	schemaTokenCurrentTime = "{{current_time}}"
	schemaTokenNow         = "{{now}}"
)

var schemaTimestampTypePattern = regexp.MustCompile(`(?i)\b(?:TIMESTAMPTZ|TIMESTAMP|DATETIME)\b`)

// SchemaTokenTimestamp is the portable timestamp type token for schema text.
const SchemaTokenTimestamp = schemaTokenTimestamp

// SchemaTokenBoolean is the portable boolean type token for schema text.
const SchemaTokenBoolean = schemaTokenBoolean

// SchemaTokenIdentity is the portable integer identity type token for schema
// text. It is used after a column name and before its constraints.
const SchemaTokenIdentity = schemaTokenIdentity

// SchemaTokenCurrentTime is the portable current-time default token.
const SchemaTokenCurrentTime = schemaTokenCurrentTime

// RenderSchema expands the small, explicit set of portable schema tokens for a
// supported driver. Unknown or unexpanded tokens are rejected so a new schema
// cannot silently ship with the wrong engine-specific type.
func RenderSchema(driver, schema string) (string, error) {
	if driver != SQLite3 && driver != PGX {
		return "", fmt.Errorf("unsupported database driver %q", driver)
	}
	timestamp := TimestampType(driver)
	boolean := "INTEGER"
	identity := "INTEGER"
	if IsPostgres(driver) {
		boolean = "BOOLEAN"
		identity = "BIGSERIAL"
	}
	rendered := strings.NewReplacer(
		schemaTokenTimestamp, timestamp,
		schemaTokenBool, boolean,
		schemaTokenBoolean, boolean,
		schemaTokenIdentity, identity,
		schemaTokenCurrentTime, "CURRENT_TIMESTAMP",
		schemaTokenNow, "CURRENT_TIMESTAMP",
	).Replace(schema)
	// Existing stores used these generic type spellings before the token
	// contract was introduced. Keep their rendering centralized while those
	// schema constants migrate to explicit tokens. Word boundaries are
	// important here: CURRENT_TIMESTAMP is an expression, not a type.
	rendered = schemaTimestampTypePattern.ReplaceAllString(rendered, timestamp)
	if IsPostgres(driver) {
		rendered = strings.ReplaceAll(rendered, "BOOLEAN NOT NULL DEFAULT 1", "BOOLEAN NOT NULL DEFAULT TRUE")
		rendered = strings.ReplaceAll(rendered, "BOOLEAN NOT NULL DEFAULT 0", "BOOLEAN NOT NULL DEFAULT FALSE")
		rendered = strings.ReplaceAll(rendered, "BOOLEAN DEFAULT 1", "BOOLEAN DEFAULT TRUE")
		rendered = strings.ReplaceAll(rendered, "BOOLEAN DEFAULT 0", "BOOLEAN DEFAULT FALSE")
	}
	if IsPostgres(driver) {
		// Apply the boolean default normalization after token expansion too.
		// A tokenized BOOLEAN column cannot be normalized by the legacy pass
		// above because its type is not known until this point.
		rendered = strings.ReplaceAll(rendered, "BOOLEAN NOT NULL DEFAULT 1", "BOOLEAN NOT NULL DEFAULT TRUE")
		rendered = strings.ReplaceAll(rendered, "BOOLEAN NOT NULL DEFAULT 0", "BOOLEAN NOT NULL DEFAULT FALSE")
		rendered = strings.ReplaceAll(rendered, "BOOLEAN DEFAULT 1", "BOOLEAN DEFAULT TRUE")
		rendered = strings.ReplaceAll(rendered, "BOOLEAN DEFAULT 0", "BOOLEAN DEFAULT FALSE")
	}
	if strings.Contains(rendered, "{{") || strings.Contains(rendered, "}}") {
		return "", fmt.Errorf("schema contains unknown or unexpanded token")
	}
	return rendered, nil
}

// MustRenderSchema is the startup convenience for static schema constants.
// A schema token error is a programming/configuration error and must not be
// silently ignored by a constructor.
func MustRenderSchema(driver, schema string) string {
	rendered, err := RenderSchema(driver, schema)
	if err != nil {
		panic(err)
	}
	return rendered
}
