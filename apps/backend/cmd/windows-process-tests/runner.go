package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const packagePattern = "./internal/agentctl/server/process/..."
const packageRoot = "github.com/kandev/kandev/internal/agentctl/server/process"

const (
	actionOutput = "output"
	actionPass   = "pass"
	actionSkip   = "skip"
	actionFail   = "fail"
)

type listedPackage struct {
	Names   []string
	NoTests bool
}
type inventory map[string]listedPackage
type cohort struct {
	Names    []string
	Selector string
}
type command interface {
	Start() error
	Wait() error
}

type testEvent struct {
	Action, Package, Test, Output string
}

func readEvents(reader io.Reader, visit func(testEvent) error) error {
	decoder := json.NewDecoder(reader)
	for {
		var event testEvent
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("decode native Go event: %w", err)
		}
		if event.Package == "" && (event.Action == "build-start" || event.Action == "build-output") {
			continue
		}
		if event.Package != packageRoot && !strings.HasPrefix(event.Package, packageRoot+"/") {
			return fmt.Errorf("unexpected native package %q", event.Package)
		}
		switch event.Action {
		case "start", "run", "pause", "cont", actionOutput, actionPass, actionSkip, actionFail:
		default:
			return fmt.Errorf("unknown native event action %q", event.Action)
		}
		if err := visit(event); err != nil {
			return err
		}
	}
}

func nativeName(name string) bool {
	if !strings.HasPrefix(name, "Test") && !strings.HasPrefix(name, "Fuzz") && !strings.HasPrefix(name, "Example") {
		return false
	}
	if name == "TestMain" {
		return false
	}
	for _, char := range name {
		if char != '_' && !unicode.IsLetter(char) && !unicode.IsDigit(char) {
			return false
		}
	}
	return true
}

func readInventory(reader io.Reader) (inventory, error) {
	state := listing{inv: inventory{}, done: map[string]bool{}}
	if err := readEvents(reader, state.accept); err != nil {
		return nil, err
	}
	for name, pkg := range state.inv {
		if !state.done[name] || (len(pkg.Names) == 0 && !pkg.NoTests) || (pkg.NoTests && len(pkg.Names) != 0) {
			return nil, fmt.Errorf("missing native inventory/completion for %s", name)
		}
	}
	if _, err := partition(state.inv); err != nil {
		return nil, err
	}
	return state.inv, nil
}

type listing struct {
	inv  inventory
	done map[string]bool
}

func (state *listing) accept(event testEvent) error {
	if event.Test != "" || state.done[event.Package] {
		return fmt.Errorf("unexpected enumeration event after completion or test execution: %+v", event)
	}
	pkg := state.inv[event.Package]
	if event.Action == actionOutput {
		if err := listOutput(&pkg, event.Output); err != nil {
			return err
		}
	}
	if event.Action == actionFail || (event.Action == actionSkip && !pkg.NoTests) {
		return fmt.Errorf("native enumeration failed or skipped: %s", event.Package)
	}
	if event.Action == actionPass || event.Action == actionSkip {
		state.done[event.Package] = true
	}
	state.inv[event.Package] = pkg
	return nil
}

func listOutput(pkg *listedPackage, output string) error {
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		line = strings.TrimSuffix(line, "\r")
		switch {
		case line == "" || line == "PASS" || strings.HasPrefix(line, "ok ") || strings.HasPrefix(line, "ok\t"):
		case strings.HasPrefix(line, "?") && strings.Contains(line, "[no test files]"):
			pkg.NoTests = true
		case nativeName(line):
			for _, existing := range pkg.Names {
				if existing == line {
					return fmt.Errorf("duplicate native name %q", line)
				}
			}
			pkg.Names = append(pkg.Names, line)
		default:
			return fmt.Errorf("unexpected native listing output %q", line)
		}
	}
	return nil
}

func partition(inv inventory) ([2]cohort, error) {
	var groups [2]cohort
	names := map[string]bool{}
	for _, pkg := range inv {
		seen := map[string]bool{}
		for _, name := range pkg.Names {
			if !nativeName(name) || seen[name] {
				return groups, fmt.Errorf("invalid or duplicate native name %q", name)
			}
			seen[name], names[name] = true, true
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	if len(ordered) < 2 {
		return groups, errors.New("native inventory cannot form two nonempty cohorts")
	}
	for index, name := range ordered {
		groups[index%2].Names = append(groups[index%2].Names, name)
	}
	for index := range groups {
		escaped := make([]string, len(groups[index].Names))
		for i, name := range groups[index].Names {
			escaped[i] = regexp.QuoteMeta(name)
		}
		groups[index].Selector = "^(" + strings.Join(escaped, "|") + ")$"
	}
	return groups, verifyPartition(inv, groups)
}

func verifyPartition(inv inventory, groups [2]cohort) error {
	selectors := [2]*regexp.Regexp{}
	for i, group := range groups {
		selectors[i] = regexp.MustCompile(group.Selector)
	}
	for _, pkg := range inv {
		for _, name := range pkg.Names {
			matches := 0
			for _, selector := range selectors {
				if selector.MatchString(name) {
					matches++
				}
			}
			if matches != 1 {
				return fmt.Errorf("native name %s matches %d cohorts", name, matches)
			}
		}
	}
	return nil
}

type coverage struct {
	inv       inventory
	selector  *regexp.Regexp
	completed map[string]bool
	packages  map[string]bool
	noTests   map[string]bool
}

func validateCoverage(reader io.Reader, inv inventory, group cohort) error {
	selector, err := regexp.Compile(group.Selector)
	if err != nil {
		return err
	}
	state := coverage{inv: inv, selector: selector, completed: map[string]bool{}, packages: map[string]bool{}, noTests: map[string]bool{}}
	if err := readEvents(reader, state.accept); err != nil {
		return err
	}
	for name, pkg := range inv {
		if !state.packages[name] {
			return fmt.Errorf("missing native package completion: %s", name)
		}
		for _, test := range pkg.Names {
			if selector.MatchString(test) && !state.completed[name+"/"+test] {
				return fmt.Errorf("missing native test completion: %s/%s", name, test)
			}
		}
	}
	return nil
}

func (state *coverage) accept(event testEvent) error {
	pkg, exists := state.inv[event.Package]
	if !exists || state.packages[event.Package] {
		return fmt.Errorf("unlisted or already completed package: %s", event.Package)
	}
	if event.Action == actionFail {
		return fmt.Errorf("native test/package failed: %s/%s", event.Package, event.Test)
	}
	if event.Test != "" {
		return state.acceptTest(event, pkg)
	}
	if event.Action == actionOutput && strings.HasPrefix(event.Output, "?") && strings.Contains(event.Output, "[no test files]") {
		state.noTests[event.Package] = true
	}
	if event.Action == actionSkip && (!pkg.NoTests || !state.noTests[event.Package]) {
		return fmt.Errorf("native package skipped without no-test evidence: %s", event.Package)
	}
	if event.Action == actionPass || event.Action == actionSkip {
		if pkg.NoTests && !state.noTests[event.Package] {
			return fmt.Errorf("missing no-test package evidence: %s", event.Package)
		}
		state.packages[event.Package] = true
	}
	return nil
}

func (state *coverage) acceptTest(event testEvent, pkg listedPackage) error {
	root := strings.SplitN(event.Test, "/", 2)[0]
	found := false
	for _, name := range pkg.Names {
		found = found || root == name
	}
	if !found || !state.selector.MatchString(root) {
		return fmt.Errorf("unselected native test: %s/%s", event.Package, event.Test)
	}
	if event.Test == root && (event.Action == actionPass || event.Action == actionSkip) {
		key := event.Package + "/" + root
		if state.completed[key] {
			return fmt.Errorf("duplicate native test completion: %s", key)
		}
		state.completed[key] = true
	}
	return nil
}

func joinCommands(commands []command) error {
	var started []command
	var failures []error
	for index, cmd := range commands {
		if err := cmd.Start(); err != nil {
			failures = append(failures, fmt.Errorf("start command %d: %w", index, err))
			continue
		}
		started = append(started, cmd)
	}
	for index, cmd := range started {
		if err := cmd.Wait(); err != nil {
			failures = append(failures, fmt.Errorf("join started command %d: %w", index, err))
		}
	}
	return errors.Join(failures...)
}

func commandArgs(selector string) []string {
	if selector == "" {
		return []string{"test", "-race", "-json", "-timeout", "25m", "-list", "^(Test|Fuzz|Example)", packagePattern}
	}
	return []string{"test", "-race", "-count=1", "-v", "-json", "-timeout", "25m", "-run", selector, packagePattern}
}
