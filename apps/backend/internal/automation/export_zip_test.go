package automation

import (
	"archive/zip"
	"bytes"
	"testing"

	"gopkg.in/yaml.v3"
)

func readZipFile(t *testing.T, data []byte, name string) ([]byte, bool) {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %q: %v", name, err)
		}
		defer func() { _ = rc.Close() }()
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatalf("read %q: %v", name, err)
		}
		return buf.Bytes(), true
	}
	return nil, false
}

func zipEntryNames(t *testing.T, data []byte) []string {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	names := make([]string, len(r.File))
	for i, f := range r.File {
		names[i] = f.Name
	}
	return names
}

// AC-24: one file per automation at .kandev/automations/<slug>.yml, each the
// full envelope with a single-element automations list.

func TestBuildAutomationZip_OneEntryPerAutomationAtSlugPath(t *testing.T) {
	entries := []automationZipEntry{
		{ID: "id-1", Name: "Daily Sync", Automation: exportAutomation{Name: "Daily Sync", Triggers: []exportTrigger{}}},
	}
	data, err := buildAutomationZip(entries, nil)
	if err != nil {
		t.Fatalf("buildAutomationZip: %v", err)
	}

	content, ok := readZipFile(t, data, ".kandev/automations/daily-sync.yml")
	if !ok {
		t.Fatalf("entry .kandev/automations/daily-sync.yml not found; entries: %v", zipEntryNames(t, data))
	}

	var doc exportDocument
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if doc.Version != exportDocumentVersion || doc.Type != exportDocumentType {
		t.Errorf("envelope = %+v, want version=%d type=%q", doc, exportDocumentVersion, exportDocumentType)
	}
	if len(doc.Automations) != 1 || doc.Automations[0].Name != "Daily Sync" {
		t.Errorf("automations = %+v, want single-element list with Daily Sync", doc.Automations)
	}
	if len(doc.Warnings) != 0 {
		t.Errorf("warnings = %v, want none embedded in a per-automation zip entry", doc.Warnings)
	}
}

func TestBuildAutomationZip_MultipleAutomations_OneFileEach(t *testing.T) {
	entries := []automationZipEntry{
		{ID: "id-1", Name: "Daily Sync", Automation: exportAutomation{Name: "Daily Sync", Triggers: []exportTrigger{}}},
		{ID: "id-2", Name: "Weekly Review", Automation: exportAutomation{Name: "Weekly Review", Triggers: []exportTrigger{}}},
	}
	data, err := buildAutomationZip(entries, nil)
	if err != nil {
		t.Fatalf("buildAutomationZip: %v", err)
	}
	names := zipEntryNames(t, data)
	want := []string{".kandev/automations/daily-sync.yml", ".kandev/automations/weekly-review.yml"}
	if len(names) != len(want) {
		t.Fatalf("got entries %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, names[i], want[i])
		}
	}
}

// AC-28: entries ordered by entry path ascending.

func TestBuildAutomationZip_EntriesOrderedByPathAscending(t *testing.T) {
	entries := []automationZipEntry{
		{ID: "id-1", Name: "Zeta", Automation: exportAutomation{Name: "Zeta", Triggers: []exportTrigger{}}},
		{ID: "id-2", Name: "Alpha", Automation: exportAutomation{Name: "Alpha", Triggers: []exportTrigger{}}},
		{ID: "id-3", Name: "Mid", Automation: exportAutomation{Name: "Mid", Triggers: []exportTrigger{}}},
	}
	data, err := buildAutomationZip(entries, nil)
	if err != nil {
		t.Fatalf("buildAutomationZip: %v", err)
	}
	names := zipEntryNames(t, data)
	want := []string{".kandev/automations/alpha.yml", ".kandev/automations/mid.yml", ".kandev/automations/zeta.yml"}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, names[i], want[i])
		}
	}
}

// AC-27: colliding automation names get unique slugs in the zip.

func TestBuildAutomationZip_CollidingNamesGetUniqueEntries(t *testing.T) {
	entries := []automationZipEntry{
		{ID: "aaaaaaaa1111", Name: "Daily Sync", Automation: exportAutomation{Name: "Daily Sync", Triggers: []exportTrigger{}}},
		{ID: "bbbbbbbb2222", Name: "Daily Sync", Automation: exportAutomation{Name: "Daily Sync", Triggers: []exportTrigger{}}},
	}
	data, err := buildAutomationZip(entries, nil)
	if err != nil {
		t.Fatalf("buildAutomationZip: %v", err)
	}
	names := zipEntryNames(t, data)
	want := []string{".kandev/automations/daily-sync-aaaaaaaa.yml", ".kandev/automations/daily-sync-bbbbbbbb.yml"}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, names[i], want[i])
		}
	}
}

// AC-20/AC-21/AC-42: warnings live in .kandev/automations/WARNINGS.txt for
// the zip form, one already-rendered line per warning, absent when there are
// none.

func TestBuildAutomationZip_NoWarnings_NoWarningsFile(t *testing.T) {
	entries := []automationZipEntry{
		{ID: "id-1", Name: "Daily Sync", Automation: exportAutomation{Name: "Daily Sync", Triggers: []exportTrigger{}}},
	}
	data, err := buildAutomationZip(entries, nil)
	if err != nil {
		t.Fatalf("buildAutomationZip: %v", err)
	}
	if _, ok := readZipFile(t, data, ".kandev/automations/WARNINGS.txt"); ok {
		t.Error("WARNINGS.txt present with no warnings; want absent")
	}
}

func TestBuildAutomationZip_WithWarnings_WritesWarningsTxt(t *testing.T) {
	entries := []automationZipEntry{
		{ID: "id-1", Name: "Daily Sync", Automation: exportAutomation{Name: "Daily Sync", Triggers: []exportTrigger{}}},
	}
	warnings := []string{"Alpha: unresolved agent profile", "Zeta: unresolved workflow"}
	data, err := buildAutomationZip(entries, warnings)
	if err != nil {
		t.Fatalf("buildAutomationZip: %v", err)
	}
	content, ok := readZipFile(t, data, ".kandev/automations/WARNINGS.txt")
	if !ok {
		t.Fatalf("WARNINGS.txt not found; entries: %v", zipEntryNames(t, data))
	}
	want := "Alpha: unresolved agent profile\nZeta: unresolved workflow\n"
	if string(content) != want {
		t.Errorf("got %q, want %q", content, want)
	}
}

func TestBuildAutomationZip_WarningsFileSortsWithOtherEntries(t *testing.T) {
	entries := []automationZipEntry{
		{ID: "id-1", Name: "9-lives", Automation: exportAutomation{Name: "9-lives", Triggers: []exportTrigger{}}},
	}
	data, err := buildAutomationZip(entries, []string{"9-lives: unresolved workflow"})
	if err != nil {
		t.Fatalf("buildAutomationZip: %v", err)
	}
	names := zipEntryNames(t, data)
	// '9' (0x39) sorts before 'W' (0x57): the digit-led slug entry comes
	// first, WARNINGS.txt second — proving no special-cased placement.
	want := []string{".kandev/automations/9-lives.yml", ".kandev/automations/WARNINGS.txt"}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, names[i], want[i])
		}
	}
}

// AC-28: no wall-clock value enters the archive — building the same input
// twice, with real time elapsing between builds, must be byte-identical.

func TestBuildAutomationZip_ReproducibleAcrossBuilds(t *testing.T) {
	entries := []automationZipEntry{
		{ID: "id-1", Name: "Daily Sync", Automation: exportAutomation{Name: "Daily Sync", Triggers: []exportTrigger{}}},
		{ID: "id-2", Name: "Weekly Review", Automation: exportAutomation{Name: "Weekly Review", Triggers: []exportTrigger{}}},
	}
	warnings := []string{"Daily Sync: unresolved agent profile"}

	first, err := buildAutomationZip(entries, warnings)
	if err != nil {
		t.Fatalf("buildAutomationZip (first): %v", err)
	}
	second, err := buildAutomationZip(entries, warnings)
	if err != nil {
		t.Fatalf("buildAutomationZip (second): %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Error("two builds of the same input produced different zip bytes")
	}
}

func TestBuildAutomationZip_EmptyInput_ProducesEmptyZip(t *testing.T) {
	data, err := buildAutomationZip(nil, nil)
	if err != nil {
		t.Fatalf("buildAutomationZip: %v", err)
	}
	if len(zipEntryNames(t, data)) != 0 {
		t.Errorf("got %v, want no entries", zipEntryNames(t, data))
	}
}
