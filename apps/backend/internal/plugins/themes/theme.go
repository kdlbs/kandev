// Package themes validates portable, data-only color themes.
package themes

type Style struct {
	Foreground string `json:"foreground,omitempty"`
	Background string `json:"background,omitempty"`
	FontStyle  string `json:"fontStyle,omitempty"`
}
type TokenRule struct {
	Scope    any   `json:"scope"`
	Settings Style `json:"settings"`
}
type Definition struct {
	SchemaVersion int               `json:"schema_version"`
	Base          string            `json:"base,omitempty"`
	UI            map[string]string `json:"ui,omitempty"`
	Editor        map[string]string `json:"editor,omitempty"`
	Syntax        map[string]Style  `json:"syntax,omitempty"`
	Terminal      map[string]string `json:"terminal,omitempty"`
	Textmate      []TokenRule       `json:"textmate,omitempty"`
	Semantic      map[string]Style  `json:"semantic,omitempty"`
}
