package configsync

import "testing"

func TestFilenameStem(t *testing.T) {
	cases := map[string]string{
		"agents/reviewer.yml":  "reviewer",
		"agents/reviewer.yaml": "reviewer",
		"skills/reviewer":      "reviewer",
		"a.b/c.yml":            "c",
	}
	for path, want := range cases {
		if got := filenameStem(path); got != want {
			t.Errorf("filenameStem(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestResolveKeyCollisions_NoCollisionNoWarning(t *testing.T) {
	winners, warnings := resolveKeyCollisions("agent", []keyedPath{
		{Key: "ceo", Path: "agents/ceo.yml"},
		{Key: "cto", Path: "agents/cto.yml"},
	})
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	if winners["ceo"] != "agents/ceo.yml" || winners["cto"] != "agents/cto.yml" {
		t.Errorf("winners = %v", winners)
	}
}

func TestResolveKeyCollisions_LexicographicallyFirstPathWins(t *testing.T) {
	winners, warnings := resolveKeyCollisions("agent", []keyedPath{
		{Key: "ceo", Path: "agents/z-dup.yml"},
		{Key: "ceo", Path: "agents/a-dup.yml"},
	})
	if winners["ceo"] != "agents/a-dup.yml" {
		t.Errorf("winner = %q, want agents/a-dup.yml", winners["ceo"])
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one", warnings)
	}
}

func TestResolveKeyCollisions_WinnerIsIndependentOfInputOrder(t *testing.T) {
	forward, _ := resolveKeyCollisions("agent", []keyedPath{
		{Key: "ceo", Path: "agents/a-dup.yml"},
		{Key: "ceo", Path: "agents/z-dup.yml"},
	})
	reverse, _ := resolveKeyCollisions("agent", []keyedPath{
		{Key: "ceo", Path: "agents/z-dup.yml"},
		{Key: "ceo", Path: "agents/a-dup.yml"},
	})
	if forward["ceo"] != reverse["ceo"] {
		t.Errorf("winner depends on input order: forward=%q reverse=%q", forward["ceo"], reverse["ceo"])
	}
}

func TestStemMismatchWarning_MatchingStemReturnsEmpty(t *testing.T) {
	if got := stemMismatchWarning("agent", "agents/ceo.yml", "ceo"); got != "" {
		t.Errorf("stemMismatchWarning() = %q, want empty for matching stem", got)
	}
}

func TestStemMismatchWarning_DifferingStemWarns(t *testing.T) {
	got := stemMismatchWarning("agent", "agents/x.yml", "CEO Agent")
	if got == "" {
		t.Fatal("stemMismatchWarning() = empty, want a warning")
	}
}
