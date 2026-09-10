package service

import "testing"

// These parsers previously only compiled under //go:build darwin, so no
// CI runner (there is no macos-latest job) ever ran them. They are pure
// string parsing untagged in resource_cleanup_orphan_reap_host_parse.go, so
// this file exercises them directly on every platform CI does run.

func TestParseLsofCwdEntries(t *testing.T) {
	input := "p111\ncbash\nn/home/a\np222\ncsh\nn/home/b\n"
	got := parseLsofCwdEntries([]byte(input))
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(got), got)
	}
	if got[111].Cwd != "/home/a" || got[111].Command != "bash" {
		t.Fatalf("unexpected entry for pid 111: %+v", got[111])
	}
	if got[222].Cwd != "/home/b" || got[222].Command != "sh" {
		t.Fatalf("unexpected entry for pid 222: %+v", got[222])
	}
}

func TestParseLsofCwdEntriesDropsMissingCwd(t *testing.T) {
	// AC-TASKS-ORPHAN-REAP-002.1: a process whose entry cannot be parsed
	// (here, no 'n' cwd line) is not a candidate.
	input := "p333\ncsh\n"
	got := parseLsofCwdEntries([]byte(input))
	if len(got) != 0 {
		t.Fatalf("expected no entries for a record missing cwd, got %+v", got)
	}
}

func TestParseLsofCwdEntriesDropsMalformedPID(t *testing.T) {
	input := "pnot-a-number\ncsh\nn/home/a\n"
	got := parseLsofCwdEntries([]byte(input))
	if len(got) != 0 {
		t.Fatalf("expected no entries for a malformed pid, got %+v", got)
	}
}

// A raw control byte inside a cwd or command value never occurs in genuine
// lsof -F output (it escapes such bytes to a printable caret form), so its
// presence signals a record this parser cannot trust the identity of --
// including one crafted to smuggle a fabricated p/c/n record into the
// stream. The whole record is dropped rather than truncated at the control
// byte, and a well-formed record elsewhere in the same input still parses.
func TestParseLsofCwdEntriesDropsRecordWithControlByteInCwd(t *testing.T) {
	input := "p111\ncbash\nn/home/a\x01evil\np222\ncsh\nn/home/b\n"
	got := parseLsofCwdEntries([]byte(input))
	if len(got) != 1 {
		t.Fatalf("expected only the well-formed record to survive, got %+v", got)
	}
	if _, ok := got[111]; ok {
		t.Fatalf("expected pid 111's record (control byte in cwd) to be dropped, got %+v", got[111])
	}
	if got[222].Cwd != "/home/b" || got[222].Command != "sh" {
		t.Fatalf("unexpected entry for pid 222: %+v", got[222])
	}
}

func TestParseLsofCwdEntriesDropsRecordWithControlByteInCommand(t *testing.T) {
	input := "p111\ncba\x07sh\nn/home/a\np222\ncsh\nn/home/b\n"
	got := parseLsofCwdEntries([]byte(input))
	if len(got) != 1 {
		t.Fatalf("expected only the well-formed record to survive, got %+v", got)
	}
	if _, ok := got[111]; ok {
		t.Fatalf("expected pid 111's record (control byte in command) to be dropped, got %+v", got[111])
	}
}

func TestParsePSAncestry(t *testing.T) {
	input := "  1   0\n  222   1\n"
	got := parsePSAncestry([]byte(input))
	if len(got) != 2 || got[1] != 0 || got[222] != 1 {
		t.Fatalf("unexpected ancestry map: %+v", got)
	}
}

func TestParsePSAncestrySkipsMalformedLines(t *testing.T) {
	input := "not-a-pid 1\n222 1\n222\n"
	got := parsePSAncestry([]byte(input))
	if len(got) != 1 || got[222] != 1 {
		t.Fatalf("expected only the well-formed line to survive, got %+v", got)
	}
}

// A pid whose cwd lsof could not resolve (so it is absent from byPID)
// must still appear in the combined snapshot with its ancestry intact, so
// the ownership walk (AC-TASKS-ORPHAN-REAP-003.3) does not lose a hop.
func TestCombineLsofAndPSSnapshotKeepsAncestryOnlyEntries(t *testing.T) {
	byPID := map[int]hostProcess{
		500: {PID: 500, Cwd: "/task/root", Command: "node"},
	}
	ppidByPID := map[int]int{
		500: 450, // 500's parent, cwd unreadable so absent from byPID
		450: 400, // 450's parent, itself another task's recorded local_pid
	}

	got := combineLsofAndPSSnapshot(byPID, ppidByPID)

	byPIDOut := make(map[int]hostProcess, len(got))
	for _, p := range got {
		byPIDOut[p.PID] = p
	}
	if len(byPIDOut) != 2 {
		t.Fatalf("expected 2 entries (one cwd-bearing, one ancestry-only), got %+v", byPIDOut)
	}
	if byPIDOut[500].PPID != 450 || byPIDOut[500].Cwd != "/task/root" {
		t.Fatalf("expected pid 500's cwd and ppid preserved, got %+v", byPIDOut[500])
	}
	ancestryOnly, ok := byPIDOut[450]
	if !ok {
		t.Fatalf("expected an ancestry-only entry for pid 450, got %+v", byPIDOut)
	}
	if ancestryOnly.PPID != 400 {
		t.Fatalf("expected pid 450's ppid to be 400, got %+v", ancestryOnly)
	}
	if ancestryOnly.Cwd != "" {
		t.Fatalf("expected an ancestry-only entry to have an empty cwd (never a candidate), got %+v", ancestryOnly)
	}
}

func TestCombineLsofAndPSSnapshotKeepsCwdOnlyEntry(t *testing.T) {
	// A pid lsof saw but ps somehow missed (race between the two commands)
	// still contributes what lsof knows, with ppid defaulting to 0.
	byPID := map[int]hostProcess{
		600: {PID: 600, Cwd: "/task/root", Command: "sh"},
	}
	got := combineLsofAndPSSnapshot(byPID, map[int]int{})
	if len(got) != 1 || got[0].PID != 600 || got[0].Cwd != "/task/root" || got[0].PPID != 0 {
		t.Fatalf("unexpected combined snapshot: %+v", got)
	}
}

// These parsers previously only compiled under //go:build linux, so no
// CI runner on a non-Linux host ever ran them directly. They are pure
// string parsing untagged in resource_cleanup_orphan_reap_host_parse.go, so
// this file exercises them directly on every platform CI does run.

func TestTrimProcCwdDeletedSuffix(t *testing.T) {
	if got := trimProcCwdDeletedSuffix("/task/root (deleted)"); got != "/task/root" {
		t.Fatalf("expected the \" (deleted)\" suffix stripped, got %q", got)
	}
}

func TestTrimProcCwdDeletedSuffixLeavesOrdinaryPathUnchanged(t *testing.T) {
	if got := trimProcCwdDeletedSuffix("/task/root"); got != "/task/root" {
		t.Fatalf("expected an ordinary path unchanged, got %q", got)
	}
}

func TestParseProcStatLine(t *testing.T) {
	ppid, command, err := parseProcStatLine("500 (node) S 400 400 400 0 -1 4194560 ...\n")
	if err != nil {
		t.Fatalf("parseProcStatLine: %v", err)
	}
	if ppid != 400 || command != "node" {
		t.Fatalf("expected ppid=400 command=node, got ppid=%d command=%q", ppid, command)
	}
}

// A command name containing a space or its own parens (e.g. "(sd-pam)" or
// "my (weird) app") must not desynchronize the field count that follows:
// comm is located between the FIRST '(' and the LAST ')', not by splitting
// on whitespace.
func TestParseProcStatLineHandlesCommandWithSpacesAndParens(t *testing.T) {
	ppid, command, err := parseProcStatLine("500 (my (weird) app) S 400 400 400 0 -1 4194560\n")
	if err != nil {
		t.Fatalf("parseProcStatLine: %v", err)
	}
	if ppid != 400 || command != "my (weird) app" {
		t.Fatalf("expected ppid=400 command=%q, got ppid=%d command=%q", "my (weird) app", ppid, command)
	}
}

func TestParseProcStatLineRejectsMissingParens(t *testing.T) {
	if _, _, err := parseProcStatLine("500 node S 400\n"); err == nil {
		t.Fatalf("expected an error for a line missing comm parens")
	}
}

func TestParseProcStatLineRejectsTooFewFieldsAfterComm(t *testing.T) {
	if _, _, err := parseProcStatLine("500 (node) S\n"); err == nil {
		t.Fatalf("expected an error for a line with too few fields after comm")
	}
}

func TestParseProcStatLineRejectsNonNumericPPID(t *testing.T) {
	if _, _, err := parseProcStatLine("500 (node) S not-a-number 400\n"); err == nil {
		t.Fatalf("expected an error for a non-numeric ppid field")
	}
}
