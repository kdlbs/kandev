package main

import "regexp"

// officeScenarioDirectiveRe matches a line-anchored directive that selects a
// registered scenario from anywhere in a prompt. Office assembles prompts
// itself (AGENTS.md, wake context), so a scenario cannot be the first line.
var officeScenarioDirectiveRe = regexp.MustCompile(`(?m)^e2e:office_scenario\(([a-z0-9-]+)\)\s*$`)

func parseOfficeScenarioDirective(prompt string) (string, bool) {
	match := officeScenarioDirectiveRe.FindStringSubmatch(prompt)
	if match == nil {
		return "", false
	}
	if _, ok := scenarioRegistry[match[1]]; !ok {
		return "", false
	}
	return match[1], true
}
