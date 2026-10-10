package db

import "strings"

var sqliteURIPathReplacer = strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23")

// EscapeSQLiteURIPath escapes characters that SQLite decodes or treats as
// delimiters in the path portion of a file URI.
func EscapeSQLiteURIPath(path string) string {
	return sqliteURIPathReplacer.Replace(path)
}
