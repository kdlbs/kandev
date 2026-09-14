package configsync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// parsedAgent is the parsed, owned-field view of one agents/*.yml file
// (AC-OFFICE-CONFIG-SYNC-003.5c). It deliberately omits id: entity identity
// is the declared name (AC-OFFICE-CONFIG-SYNC-003.1), never a repository
// value.
type parsedAgent struct {
	sourcePath            string
	declaredName          string
	role                  string
	icon                  string
	reportsTo             string
	budgetMonthlyCents    int
	maxConcurrentSessions int
	desiredSkills         string
	executorPreference    string
}

type agentFileYAML struct {
	Name                  string `yaml:"name"`
	Role                  string `yaml:"role"`
	Icon                  string `yaml:"icon,omitempty"`
	ReportsTo             string `yaml:"reports_to,omitempty"`
	BudgetMonthlyCents    int    `yaml:"budget_monthly_cents,omitempty"`
	MaxConcurrentSessions int    `yaml:"max_concurrent_sessions,omitempty"`
	DesiredSkills         string `yaml:"desired_skills,omitempty"`
	ExecutorPreference    string `yaml:"executor_preference,omitempty"`
}

// parseAgentFile parses one agents/*.yml file. A missing or blank name is a
// parse failure (AC-OFFICE-CONFIG-SYNC-003.12): there is no declared name to
// key the entity by.
func parseAgentFile(sourcePath string, content []byte) (*parsedAgent, error) {
	var y agentFileYAML
	if err := yaml.Unmarshal(content, &y); err != nil {
		return nil, fmt.Errorf("parse agent yaml: %w", err)
	}
	name := strings.TrimSpace(y.Name)
	if name == "" {
		return nil, fmt.Errorf("agent file has no name")
	}
	return &parsedAgent{
		sourcePath:            sourcePath,
		declaredName:          name,
		role:                  y.Role,
		icon:                  y.Icon,
		reportsTo:             strings.TrimSpace(y.ReportsTo),
		budgetMonthlyCents:    y.BudgetMonthlyCents,
		maxConcurrentSessions: y.MaxConcurrentSessions,
		desiredSkills:         y.DesiredSkills,
		executorPreference:    y.ExecutorPreference,
	}, nil
}

// parsedProject is the parsed, owned-field view of one projects/*.yml file.
type parsedProject struct {
	sourcePath     string
	declaredName   string
	description    string
	color          string
	budgetCents    int
	repositories   string
	executorConfig string
}

type projectFileYAML struct {
	Name           string `yaml:"name"`
	Description    string `yaml:"description,omitempty"`
	Color          string `yaml:"color,omitempty"`
	BudgetCents    int    `yaml:"budget_cents,omitempty"`
	Repositories   string `yaml:"repositories,omitempty"`
	ExecutorConfig string `yaml:"executor_config,omitempty"`
}

func parseProjectFile(sourcePath string, content []byte) (*parsedProject, error) {
	var y projectFileYAML
	if err := yaml.Unmarshal(content, &y); err != nil {
		return nil, fmt.Errorf("parse project yaml: %w", err)
	}
	name := strings.TrimSpace(y.Name)
	if name == "" {
		return nil, fmt.Errorf("project file has no name")
	}
	return &parsedProject{
		sourcePath:     sourcePath,
		declaredName:   name,
		description:    y.Description,
		color:          y.Color,
		budgetCents:    y.BudgetCents,
		repositories:   y.Repositories,
		executorConfig: y.ExecutorConfig,
	}, nil
}

// parsedRoutine is the parsed, owned-field view of one routines/*.yml file.
type parsedRoutine struct {
	sourcePath        string
	declaredName      string
	description       string
	taskTemplate      string
	concurrencyPolicy string
}

type routineFileYAML struct {
	Name              string `yaml:"name"`
	Description       string `yaml:"description,omitempty"`
	TaskTemplate      string `yaml:"task_template,omitempty"`
	ConcurrencyPolicy string `yaml:"concurrency_policy,omitempty"`
}

func parseRoutineFile(sourcePath string, content []byte) (*parsedRoutine, error) {
	var y routineFileYAML
	if err := yaml.Unmarshal(content, &y); err != nil {
		return nil, fmt.Errorf("parse routine yaml: %w", err)
	}
	name := strings.TrimSpace(y.Name)
	if name == "" {
		return nil, fmt.Errorf("routine file has no name")
	}
	return &parsedRoutine{
		sourcePath:        sourcePath,
		declaredName:      name,
		description:       y.Description,
		taskTemplate:      y.TaskTemplate,
		concurrencyPolicy: y.ConcurrencyPolicy,
	}, nil
}

// parsedSkill is the parsed, owned-field view of one skills/<dir> directory.
type parsedSkill struct {
	dirName       string
	sourcePath    string // the skill's directory path (AC-OFFICE-CONFIG-SYNC-003.9d)
	name          string
	description   string
	content       string
	fileInventory string
}

// skillFrontmatterYAML is the subset of SKILL.md's optional `---`-delimited
// frontmatter block this package reads. It mirrors
// internal/office/skills.skillFrontmatter's name/description shape without
// that package's system-skill-only `kandev:` block, which sync never reads.
type skillFrontmatterYAML struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// inventoryFile is one entry of a skill's file_inventory column: every file
// found directly under the skill's references/ directory
// (AC-OFFICE-CONFIG-SYNC-002.1a). The shape mirrors the bundled system-skill
// inventory format that internal/office/skills/runtime_adapter.go already
// consumes (path + content are load-bearing; size/sha256 are informational).
type inventoryFile struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
	Content string `json:"content,omitempty"`
}

// parseSkill builds a skill's owned-field projection from its walked files.
// A nil, nil return means the directory defines no skill
// (AC-OFFICE-CONFIG-SYNC-003.2a): the caller warns and applies nothing for
// it, distinct from a parse failure, which leaves a previously applied skill
// untouched instead.
func parseSkill(sf skillFiles) (*parsedSkill, error) {
	if sf.skillMD == nil {
		return nil, nil
	}
	content := string(sf.skillMD.content)
	name, description := parseSkillFrontmatter(content)
	if name == "" {
		name = sf.dirName
	}
	inventory, err := buildFileInventory(sf.dirPath, sf.references)
	if err != nil {
		return nil, fmt.Errorf("build file inventory: %w", err)
	}
	return &parsedSkill{
		dirName:       sf.dirName,
		sourcePath:    sf.dirPath,
		name:          name,
		description:   description,
		content:       content,
		fileInventory: inventory,
	}, nil
}

// parseSkillFrontmatter reads SKILL.md's optional name/description
// frontmatter. Missing or malformed frontmatter is not a parse failure —
// name falls back to the directory name and description to empty.
func parseSkillFrontmatter(content string) (name, description string) {
	yamlPart, ok := splitFrontmatter(content)
	if !ok {
		return "", ""
	}
	var fm skillFrontmatterYAML
	if err := yaml.Unmarshal([]byte(yamlPart), &fm); err != nil {
		return "", ""
	}
	return strings.TrimSpace(fm.Name), fm.Description
}

// splitFrontmatter returns the YAML payload of a `---`-delimited frontmatter
// block opening a SKILL.md file. Mirrors
// internal/office/skills.splitFrontmatter's delimiter handling exactly (that
// function is unexported in a different package, so the algorithm is
// reimplemented here rather than shared).
func splitFrontmatter(content string) (string, bool) {
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return "", false
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(content, "---\r\n"), "---\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// buildFileInventory renders a skill's references/ files as the JSON array
// stored in the file_inventory column, sorted by path (the caller's
// references slice is already sorted by walk.go). Each entry's Path is
// relative to the skill directory (e.g. "references/checklist.md"), which is
// the form internal/agent/runtime/lifecycle/skill's materializer joins
// against a skill's local directory — the fetched files' full repository
// paths are not usable there.
func buildFileInventory(skillDirPath string, references []fetchedFile) (string, error) {
	if len(references) == 0 {
		return "[]", nil
	}
	prefix := skillDirPath + "/"
	files := make([]inventoryFile, 0, len(references))
	for _, ref := range references {
		sum := sha256.Sum256(ref.content)
		files = append(files, inventoryFile{
			Path:    strings.TrimPrefix(ref.path, prefix),
			Size:    int64(len(ref.content)),
			SHA256:  hex.EncodeToString(sum[:]),
			Content: string(ref.content),
		})
	}
	data, err := json.Marshal(files)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
