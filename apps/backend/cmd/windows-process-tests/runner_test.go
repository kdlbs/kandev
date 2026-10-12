package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func nativeList() string {
	return eventStream(
		map[string]string{"Action": "start", "Package": packageRoot},
		map[string]string{"Action": "output", "Package": packageRoot, "Output": "TestAlpha\nTestAlphaLong\nFuzzSeeds\nExampleSample\n"},
		map[string]string{"Action": "pass", "Package": packageRoot},
		map[string]string{"Action": "start", "Package": packageRoot + "/probe"},
		map[string]string{"Action": "output", "Package": packageRoot + "/probe", "Output": "TestAlpha\n"},
		map[string]string{"Action": "pass", "Package": packageRoot + "/probe"},
		map[string]string{"Action": "start", "Package": packageRoot + "/empty"},
		map[string]string{"Action": "output", "Package": packageRoot + "/empty", "Output": "?\t" + packageRoot + "/empty\t[no test files]\n"},
		map[string]string{"Action": "skip", "Package": packageRoot + "/empty"},
	)
}

func eventStream(events ...map[string]string) string {
	var output strings.Builder
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			panic(err)
		}
		output.Write(data)
		output.WriteByte('\n')
	}
	return output.String()
}

// @covers AC-PLATFORM-CI-PERFORMANCE-003.4
func TestCohortNativeListParsing(t *testing.T) {
	inv, err := readInventory(strings.NewReader(nativeList()))
	if err != nil {
		t.Fatalf("valid native enumeration: %v", err)
	}
	if len(inv) != 3 || len(inv[packageRoot].Names) != 4 || !inv[packageRoot+"/empty"].NoTests {
		t.Fatalf("incomplete native inventory: %+v", inv)
	}
	for name, stream := range map[string]string{
		"duplicate":                strings.Replace(nativeList(), "TestAlpha\\nTestAlphaLong", "TestAlpha\\nTestAlpha\\nTestAlphaLong", 1),
		"unknown package":          strings.ReplaceAll(nativeList(), packageRoot, "example.invalid/other"),
		"malformed":                nativeList() + "{",
		"missing package terminal": strings.Replace(nativeList(), `"Action":"pass"`, `"Action":"output"`, 1),
		"failed enumeration":       strings.Replace(nativeList(), `"Action":"pass"`, `"Action":"fail"`, 1),
		"skipped inventory":        strings.Replace(nativeList(), `"Action":"pass"`, `"Action":"skip"`, 1),
		"invalid name":             strings.Replace(nativeList(), "TestAlphaLong", "TestAlpha/nested", 1),
		"unknown output":           strings.Replace(nativeList(), "TestAlphaLong", "not a native test name", 1),
		"unknown action":           strings.Replace(nativeList(), `"Action":"start"`, `"Action":"unknown"`, 1),
		"empty":                    "",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := readInventory(strings.NewReader(stream)); err == nil {
				t.Fatal("invalid enumeration certified an inventory")
			}
		})
	}
}

func sampleInventory() inventory {
	return inventory{
		packageRoot:            {Names: []string{"TestAlphaLong", "TestAlpha", "FuzzSeeds", "ExampleSample"}},
		packageRoot + "/probe": {Names: []string{"TestAlpha"}},
		packageRoot + "/empty": {NoTests: true},
	}
}

func TestCohortPartition(t *testing.T) {
	cohorts, err := partition(sampleInventory())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cohorts[0].Names, []string{"ExampleSample", "TestAlpha"}) ||
		!reflect.DeepEqual(cohorts[1].Names, []string{"FuzzSeeds", "TestAlphaLong"}) {
		t.Fatalf("non-deterministic or incomplete partition: %+v", cohorts)
	}
	shuffled := sampleInventory()
	shuffled[packageRoot] = listedPackage{Names: []string{"ExampleSample", "FuzzSeeds", "TestAlpha", "TestAlphaLong"}}
	again, err := partition(shuffled)
	if err != nil || !reflect.DeepEqual(again, cohorts) {
		t.Fatalf("input order changes assignment: %+v, %v", again, err)
	}
	for _, inv := range []inventory{nil, {packageRoot: {Names: []string{"TestOnly"}}}, {packageRoot: {Names: []string{"TestA", "TestA"}}}} {
		if _, err := partition(inv); err == nil {
			t.Fatal("empty, overlapping or empty-cohort partition accepted")
		}
	}
}

func TestCohortSelectors(t *testing.T) {
	if os.Getenv("KANDEV_COHORT_SELECTOR_FIXTURE") == "1" && os.Args[len(os.Args)-1] == "selector-fixture" {
		for _, name := range []string{"first/nested", "second"} {
			t.Run(name, func(t *testing.T) {
				if _, err := fmt.Fprintln(os.Stdout, "selected "+name); err != nil {
					t.Fatal(err)
				}
			})
		}
		return
	}
	cohorts, err := partition(sampleInventory())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ExampleSample", "FuzzSeeds", "TestAlpha", "TestAlphaLong"} {
		matches := 0
		for _, group := range cohorts {
			matches += boolInt(regexp.MustCompile(group.Selector).MatchString(name))
		}
		if matches != 1 {
			t.Fatalf("%s matches %d cohorts", name, matches)
		}
	}
	if regexp.MustCompile(cohorts[0].Selector).MatchString("TestAlphaLong") {
		t.Fatal("rooted selector selected a sibling by prefix")
	}
	fixture, err := partition(inventory{packageRoot: {Names: []string{"TestCohortSelectors", "TestExcluded"}}})
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run", fixture[0].Selector, "-test.timeout=1m", "--", "selector-fixture")
	cmd.Env = append(os.Environ(), "KANDEV_COHORT_SELECTOR_FIXTURE=1")
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "selected first/nested") || !strings.Contains(string(output), "selected second") {
		t.Fatalf("native root selector lost subtests: %s, %v", output, err)
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type observedCommand struct {
	index             int
	events            *[]string
	startErr, waitErr error
}

func (cmd *observedCommand) Start() error {
	*cmd.events = append(*cmd.events, fmt.Sprintf("start%d", cmd.index))
	return cmd.startErr
}

func (cmd *observedCommand) Wait() error {
	*cmd.events = append(*cmd.events, fmt.Sprintf("wait%d", cmd.index))
	return cmd.waitErr
}

// @covers AC-PLATFORM-CI-PERFORMANCE-003.5
func TestCohortJoinAllStartedCommands(t *testing.T) {
	failure := errors.New("native command failed")
	for _, test := range []struct {
		name                         string
		start, firstWait, secondWait error
		want                         []string
	}{
		{"success", nil, nil, nil, []string{"start0", "start1", "wait0", "wait1"}},
		{"second start", failure, nil, nil, []string{"start0", "start1", "wait0"}},
		{"first wait", nil, failure, nil, []string{"start0", "start1", "wait0", "wait1"}},
		{"both waits", nil, failure, failure, []string{"start0", "start1", "wait0", "wait1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var events []string
			commands := []command{
				&observedCommand{index: 0, events: &events, waitErr: test.firstWait},
				&observedCommand{index: 1, events: &events, startErr: test.start, waitErr: test.secondWait},
			}
			err := joinCommands(commands)
			wantError := test.start != nil || test.firstWait != nil || test.secondWait != nil
			if (err != nil) != wantError || !reflect.DeepEqual(events, test.want) {
				t.Fatalf("lost native ownership/error: events %v, error %v", events, err)
			}
		})
	}
	t.Run("actual native starts and joins", func(t *testing.T) {
		directory := t.TempDir()
		first, second := newCommand(directory, "first", ""), newCommand(directory, "second", "")
		first.Args, second.Args = []string{"version"}, []string{"not-a-go-command"}
		if err := joinCommands([]command{first, second}); err == nil {
			t.Fatal("actual failed native command certified success")
		}
		for _, cmd := range []*recordedCommand{first, second} {
			if cmd.PID <= 0 || cmd.StartedUTC == "" || cmd.JoinedUTC == "" || cmd.files != nil {
				t.Fatalf("actual command not joined and closed: %+v", cmd)
			}
		}
		if first.ExitCode != 0 || second.ExitCode != 2 {
			t.Fatalf("lost actual exits: %d / %d", first.ExitCode, second.ExitCode)
		}
	})
}

func coverageStream() string {
	return eventStream(
		map[string]string{"Action": "run", "Package": packageRoot, "Test": "TestAlpha"},
		map[string]string{"Action": "pass", "Package": packageRoot, "Test": "TestAlpha/nested"},
		map[string]string{"Action": "pass", "Package": packageRoot, "Test": "TestAlpha"},
		map[string]string{"Action": "skip", "Package": packageRoot, "Test": "ExampleSample"},
		map[string]string{"Action": "pass", "Package": packageRoot},
		map[string]string{"Action": "skip", "Package": packageRoot + "/probe", "Test": "TestAlpha"},
		map[string]string{"Action": "pass", "Package": packageRoot + "/probe"},
		map[string]string{"Action": "output", "Package": packageRoot + "/empty", "Output": "?\t" + packageRoot + "/empty\t[no test files]\n"},
		map[string]string{"Action": "skip", "Package": packageRoot + "/empty"},
	)
}

func TestCohortCoverage(t *testing.T) {
	group := cohort{Names: []string{"ExampleSample", "TestAlpha"}, Selector: "^(ExampleSample|TestAlpha)$"}
	if err := validateCoverage(strings.NewReader(coverageStream()), sampleInventory(), group); err != nil {
		t.Fatalf("complete real-shaped coverage rejected: %v", err)
	}
	for name, stream := range map[string]string{
		"missing":                 strings.Replace(coverageStream(), `"Action":"pass","Package":"`+packageRoot+`","Test":"TestAlpha"`, `"Action":"output","Package":"`+packageRoot+`","Test":"TestAlpha"`, 1),
		"duplicate":               coverageStream() + eventStream(map[string]string{"Action": "pass", "Package": packageRoot, "Test": "TestAlpha"}),
		"unselected":              strings.ReplaceAll(coverageStream(), "TestAlpha", "TestAlphaLong"),
		"failed test":             strings.Replace(coverageStream(), `"Action":"pass"`, `"Action":"fail"`, 1),
		"skipped package":         strings.Replace(coverageStream(), `"Action":"pass","Package":"`+packageRoot+`"}`, `"Action":"skip","Package":"`+packageRoot+`"}`, 1),
		"missing no-test package": strings.ReplaceAll(coverageStream(), packageRoot+"/empty", packageRoot+"/other"),
		"malformed":               coverageStream() + "{",
		"unknown action":          strings.Replace(coverageStream(), `"Action":"run"`, `"Action":"unknown"`, 1),
		"unreadable":              "",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateCoverage(strings.NewReader(stream), sampleInventory(), group); err == nil {
				t.Fatal("incomplete/failed native evidence certified success")
			}
		})
	}
	if err := validateCoverage(failedReader{}, sampleInventory(), group); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("diagnostic read error lost: %v", err)
	}
}

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestCohortRunnerCommand(t *testing.T) {
	want := []string{"test", "-race", "-count=1", "-v", "-json", "-timeout", "25m", "-run", "^(TestAlpha)$", packagePattern}
	if got := commandArgs("^(TestAlpha)$"); !reflect.DeepEqual(got, want) {
		t.Fatalf("race/timeout/selection contract changed: %v", got)
	}
	want = []string{"test", "-race", "-json", "-timeout", "25m", "-list", "^(Test|Fuzz|Example)", packagePattern}
	if got := commandArgs(""); !reflect.DeepEqual(got, want) {
		t.Fatalf("native enumeration contract changed: %v", got)
	}
}
