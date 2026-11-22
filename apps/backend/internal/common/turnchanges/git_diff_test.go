package turnchanges

import (
	"bytes"
	"testing"
)

func TestParseRawDiffPreservesRenameAndLiteralPathBytes(t *testing.T) {
	oldOID := bytes.Repeat([]byte("a"), 40)
	newOID := bytes.Repeat([]byte("b"), 40)
	output := bytes.Join([][]byte{
		append(append([]byte(":100644 100755 "+string(oldOID)+" "+string(newOID)+" R100"), 0), []byte("old\tname")...),
		[]byte("new\\name"),
		[]byte(":000000 100644 " + string(bytes.Repeat([]byte("0"), 40)) + " " + string(newOID) + " A"),
		[]byte("added\nname"),
	}, []byte{0})
	output = append(output, 0)
	changes, err := ParseRawDiff(output)
	if err != nil {
		t.Fatalf("ParseRawDiff() error = %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("ParseRawDiff() returned %d changes, want two", len(changes))
	}
	if changes[0].Status != "R100" || !bytes.Equal(changes[0].OldPath, []byte("old\tname")) || !bytes.Equal(changes[0].Path, []byte("new\\name")) {
		t.Fatalf("rename = %#v", changes[0])
	}
	if changes[0].OldMode != "100644" || changes[0].NewMode != "100755" || changes[0].NewOID != string(newOID) {
		t.Fatalf("rename metadata = %#v", changes[0])
	}
	if !bytes.Equal(changes[1].Path, []byte("added\nname")) || changes[1].Status != "A" {
		t.Fatalf("added path = %#v", changes[1])
	}
}

func TestParseRawDiffRejectsMalformedAndTruncatedRecords(t *testing.T) {
	for _, output := range [][]byte{
		[]byte(":100644 100644 aa bb M\x00file\x00"),
		[]byte(":100644 100644 " + string(bytes.Repeat([]byte("a"), 40)) + " " + string(bytes.Repeat([]byte("b"), 40)) + " R100\x00old\x00"),
		[]byte(":100644 100644 " + string(bytes.Repeat([]byte("a"), 40)) + " " + string(bytes.Repeat([]byte("b"), 40)) + " M\x00"),
	} {
		if _, err := ParseRawDiff(output); err == nil {
			t.Errorf("ParseRawDiff(%q) accepted malformed output", output)
		}
	}
}

func TestParseNumstatPreservesBinaryNullCountsAndRenamePaths(t *testing.T) {
	output := []byte("0\t0\tmode-only\x00-\t-\tbinary\x002\t0\t\x00old\tname\x00new\\name\x00")
	changes, err := ParseNumstat(output)
	if err != nil {
		t.Fatalf("ParseNumstat() error = %v", err)
	}
	if len(changes) != 3 {
		t.Fatalf("ParseNumstat() returned %d changes, want three", len(changes))
	}
	if changes[0].Added == nil || *changes[0].Added != 0 || changes[0].Deleted == nil || *changes[0].Deleted != 0 {
		t.Fatalf("known zero counts = %#v", changes[0])
	}
	if !changes[1].Binary || changes[1].Added != nil || changes[1].Deleted != nil {
		t.Fatalf("binary counts = %#v", changes[1])
	}
	if string(changes[2].OldPath) != "old\tname" || string(changes[2].Path) != "new\\name" || *changes[2].Added != 2 || *changes[2].Deleted != 0 {
		t.Fatalf("rename numstat = %#v", changes[2])
	}
}

func TestParseNumstatRejectsMalformedOutput(t *testing.T) {
	for _, output := range [][]byte{
		[]byte("1\t0\tpath"),
		[]byte("-\t0\tpath\x00"),
		[]byte("-1\t0\tpath\x00"),
		[]byte("0\t0\t\x00old\x00"),
	} {
		if _, err := ParseNumstat(output); err == nil {
			t.Errorf("ParseNumstat(%q) accepted malformed output", output)
		}
	}
}
