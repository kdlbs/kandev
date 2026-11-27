package main

import "fmt"

func selectCohorts(groups []cohort, selected int) ([]cohort, error) {
	if selected < 0 || selected > len(groups) {
		return nil, fmt.Errorf("cohort must be 0 (all) or between 1 and %d, got %d", len(groups), selected)
	}
	if selected == 0 {
		return groups, nil
	}
	return groups[selected-1 : selected], nil
}
