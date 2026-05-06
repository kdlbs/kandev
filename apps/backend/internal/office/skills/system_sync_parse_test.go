package skills

import (
	"strings"
	"testing"
)

// TestParseSystemSkill_NegativePaths covers what happens when a
// bundled SKILL.md is malformed. The current parser contract is
// MIXED:
//   - YAML syntax errors inside the frontmatter block are fail-loud
//     (parseSystemSkill returns a non-nil error wrapping "yaml: ...").
//   - Structurally-missing frontmatter (no leading `---`, empty file,
//     unterminated block) is a silent skip: parseSystemSkill returns
//     (nil, nil) so the bundle walker drops the file without aborting
//     the whole sync.
//   - The `name` field is optional — the parser falls back to the
//     slug when it's empty. The mandatory marker for a system skill
//     is the `kandev.system: true` sub-block; without it the file is
//     silently skipped too.
//
// These cases pin both halves of that contract so a future refactor
// can't quietly flip one of them.
func TestParseSystemSkill_NegativePaths(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantSpec    bool   // true → expect a non-nil spec
		wantErr     bool   // true → expect a non-nil error
		errContains string // substring required in err.Error() when wantErr
	}{
		{
			name: "malformed yaml in frontmatter",
			raw: "---\n" +
				"name: \"unterminated\n" + // unterminated quoted string
				"kandev:\n" +
				"  system: true\n" +
				"---\n" +
				"body\n",
			wantErr:     true,
			errContains: "yaml",
		},
		{
			name: "malformed yaml bad indent under kandev",
			raw: "---\n" +
				"name: ok\n" +
				"kandev:\n" +
				"system: true\n" + // bad indent: should be under kandev mapping
				"  version: 1\n" + // indented, makes the doc structurally invalid
				"---\n" +
				"body\n",
			wantErr:     true,
			errContains: "yaml",
		},
		{
			name:     "missing kandev block — silently skipped",
			raw:      "---\nname: just-a-user-skill\ndescription: hi\n---\nbody\n",
			wantSpec: false,
			wantErr:  false,
		},
		{
			name: "kandev.system false — silently skipped",
			raw: "---\n" +
				"name: not-system\n" +
				"kandev:\n" +
				"  system: false\n" +
				"---\nbody\n",
			wantSpec: false,
			wantErr:  false,
		},
		{
			name:     "no frontmatter at all — silently skipped",
			raw:      "# Just a markdown body\nno yaml here\n",
			wantSpec: false,
			wantErr:  false,
		},
		{
			name:     "empty file — silently skipped",
			raw:      "",
			wantSpec: false,
			wantErr:  false,
		},
		{
			name:     "unterminated frontmatter block — silently skipped",
			raw:      "---\nname: oops\nkandev:\n  system: true\n", // no closing ---
			wantSpec: false,
			wantErr:  false,
		},
		{
			name: "missing name with kandev.system true — uses slug fallback (no error)",
			raw: "---\n" +
				"description: no name field\n" +
				"kandev:\n" +
				"  system: true\n" +
				"  version: 1.0.0\n" +
				"---\nbody\n",
			wantSpec: true,
			wantErr:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := parseSystemSkill("test-slug", []byte(tc.raw))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (spec=%+v)", spec)
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.errContains)
				}
				if spec != nil {
					t.Errorf("expected nil spec on error, got %+v", spec)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantSpec && spec == nil {
				t.Fatalf("expected non-nil spec, got nil")
			}
			if !tc.wantSpec && spec != nil {
				t.Fatalf("expected nil spec (silent skip), got %+v", spec)
			}
			// Slug-fallback assertion for the "missing name" case.
			if tc.wantSpec && spec != nil && spec.Name != "test-slug" {
				t.Errorf("expected name to fall back to slug %q, got %q",
					"test-slug", spec.Name)
			}
		})
	}
}

// TestParseSystemSkill_HappyPathPopulatesAllFields complements the
// negative cases: a well-formed SKILL.md with the kandev block produces
// a spec carrying every parsed field plus a stable content hash. Kept
// next to the negative-path table so the two contracts read together.
func TestParseSystemSkill_HappyPathPopulatesAllFields(t *testing.T) {
	raw := "---\n" +
		"name: Demo Skill\n" +
		"description: a demo\n" +
		"kandev:\n" +
		"  system: true\n" +
		"  version: 1.2.3\n" +
		"  default_for_roles:\n" +
		"    - engineer\n" +
		"    - reviewer\n" +
		"---\n" +
		"## Body\n"

	spec, err := parseSystemSkill("demo", []byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec == nil {
		t.Fatal("expected non-nil spec")
	}
	if spec.Slug != "demo" {
		t.Errorf("slug = %q, want demo", spec.Slug)
	}
	if spec.Name != "Demo Skill" {
		t.Errorf("name = %q, want Demo Skill", spec.Name)
	}
	if spec.Description != "a demo" {
		t.Errorf("description = %q", spec.Description)
	}
	if spec.Version != "1.2.3" {
		t.Errorf("version = %q", spec.Version)
	}
	if len(spec.DefaultForRoles) != 2 ||
		spec.DefaultForRoles[0] != "engineer" ||
		spec.DefaultForRoles[1] != "reviewer" {
		t.Errorf("default_for_roles = %v", spec.DefaultForRoles)
	}
	if !strings.Contains(spec.Content, "## Body") {
		t.Errorf("body not preserved: %q", spec.Content)
	}
	if spec.ContentHash == "" {
		t.Error("content_hash must be populated")
	}
}

// TestLoadBundledSystemSkills_FailsLoudOnEmbeddedYAMLError documents
// the loader's contract w.r.t. parse errors: because parseSystemSkill
// returns a non-nil error on malformed YAML, LoadBundledSystemSkills
// would wrap and propagate that error rather than log-and-skip. We
// can't inject a malformed file into the //go:embed FS, but we can
// pin the contract by calling LoadBundledSystemSkills against the
// real embed (which must succeed — every committed SKILL.md must
// parse) and asserting both that the success path returns specs and
// that every spec the loader returns is well-formed (non-empty slug,
// non-empty content_hash). If anyone adds a malformed SKILL.md to the
// embed, this test will fail fast at the loader, matching the
// fail-loud half of the parser contract.
func TestLoadBundledSystemSkills_FailsLoudContract(t *testing.T) {
	specs, err := LoadBundledSystemSkills()
	if err != nil {
		t.Fatalf("LoadBundledSystemSkills returned error: %v", err)
	}
	if len(specs) == 0 {
		t.Skip("no bundled system skills in this build — nothing to assert")
	}
	for _, s := range specs {
		if s.Slug == "" {
			t.Errorf("loader returned spec with empty slug: %+v", s)
		}
		if s.ContentHash == "" {
			t.Errorf("loader returned spec %q with empty content_hash", s.Slug)
		}
		if s.Name == "" {
			t.Errorf("loader returned spec %q with empty name", s.Slug)
		}
	}
}
