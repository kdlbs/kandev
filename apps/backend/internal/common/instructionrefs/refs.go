// Package instructionrefs rewrites relative sibling references inside
// office instruction files (`./HEARTBEAT.md`, `./SOUL.md`, ...) to
// absolute paths under the materialised instructions directory. Both
// the office prompt builder and the runtime-tier skill deployer use
// the same rewrite so the prompt and the on-disk artifact agree, and
// the agent never has to resolve a path itself.
package instructionrefs

import "regexp"

// relativeMdRef matches `./<filename>.md` where filename is a single
// path component (no slashes). Group 1 is the leading boundary
// character (start-of-string or a non-path char), group 2 is the
// filename. The leading boundary stops `../FOO.md` from matching its
// `./FOO.md` suffix — Go's regexp has no look-behind so we capture
// and re-emit the boundary in the replacement.
var relativeMdRef = regexp.MustCompile(`(^|[^A-Za-z0-9_./])\./([A-Za-z0-9_-]+\.md)`)

// Rewrite replaces each `./<filename>.md` reference inside content
// with `<dir>/<filename>.md`. When dir is empty the content is
// returned unchanged so callers don't have to special-case it.
func Rewrite(content, dir string) string {
	if dir == "" || content == "" {
		return content
	}
	return relativeMdRef.ReplaceAllString(content, "${1}"+dir+"/$2")
}
