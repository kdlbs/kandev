package process

import "bytes"

// containsDSRQuery checks if data contains a Device Status Report (cursor position) query.
// DSR query is: ESC [ 6 n or ESC [ ? 6 n
func containsDSRQuery(data []byte) bool {
	return bytes.Contains(data, []byte("\x1b[6n")) || bytes.Contains(data, []byte("\x1b[?6n"))
}

// containsDA1Query checks if data contains a Primary Device Attributes query.
// DA1 query is: ESC [ c or ESC [ 0 c (but NOT ESC [ <digit> c where digit is 1-9,
// since those are cursor forward sequences).
func containsDA1Query(data []byte) bool {
	for i := 0; i+2 < len(data); i++ {
		if data[i] != '\x1b' || data[i+1] != '[' {
			continue
		}
		// ESC [ c — DA1 with no parameter
		if data[i+2] == 'c' {
			return true
		}
		// ESC [ 0 c — DA1 with explicit 0 parameter
		if data[i+2] == '0' && i+3 < len(data) && data[i+3] == 'c' {
			return true
		}
		// ESC [ <1-9> c would be cursor forward — skip it
	}
	return false
}
