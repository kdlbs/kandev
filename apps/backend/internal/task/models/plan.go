package models

import "unicode/utf8"

// PlanContentLength returns the Unicode code point count used by plan history.
func PlanContentLength(content string) int {
	return utf8.RuneCountInString(content)
}
