package main

import (
	"reflect"
	"testing"
)

func TestSelectedCohortRetainsNativePartitionAndRejectsInvalidSelection(t *testing.T) {
	groups, err := partition(sampleInventory(), 2)
	if err != nil {
		t.Fatal(err)
	}
	both, err := selectCohorts(groups, 0)
	if err != nil || !reflect.DeepEqual(both, groups) {
		t.Fatalf("local mode lost a cohort: %v, %v", both, err)
	}
	for index := 1; index <= 2; index++ {
		selected, err := selectCohorts(groups, index)
		if err != nil || !reflect.DeepEqual(selected, groups[index-1:index]) {
			t.Fatalf("hosted cohort %d changed selection: %v, %v", index, selected, err)
		}
	}
	for _, index := range []int{-1, 3} {
		if _, err := selectCohorts(groups, index); err == nil {
			t.Fatalf("invalid cohort %d accepted", index)
		}
	}
}

func TestFourHostedCohortsPreserveEveryNativeOccurrence(t *testing.T) {
	inv := sampleInventory()
	groups, err := partition(inv, 4)
	if err != nil || len(groups) != 4 {
		t.Fatalf("expected four nonempty hosted cohorts: %v, %v", groups, err)
	}
	for _, group := range groups {
		if len(group.Names) != 1 {
			t.Fatalf("unexpected native partition: %+v", groups)
		}
	}
	if err := verifyPartition(inv, groups); err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 4; index++ {
		selected, err := selectCohorts(groups, index)
		if err != nil || !reflect.DeepEqual(selected, groups[index-1:index]) {
			t.Fatalf("hosted cohort %d changed selection: %v, %v", index, selected, err)
		}
	}
	for _, count := range []int{0, 1, 3, 5} {
		if _, err := partition(inv, count); err == nil {
			t.Fatalf("unsupported cohort count %d accepted", count)
		}
	}
	if _, err := partition(inventory{packageRoot: {Names: []string{"TestA", "TestB"}}}, 4); err == nil {
		t.Fatal("accepted empty hosted cohorts")
	}
}
