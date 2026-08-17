package managedruntime

import (
	"fmt"
	"slices"
	"testing"
)

func TestParseStableVersion(t *testing.T) {
	for _, value := range []string{"1.2.3", "0.0.1", "10.20.30+build.7"} {
		if _, err := ParseStableVersion(value); err != nil {
			t.Errorf("ParseStableVersion(%q) = %v, want nil", value, err)
		}
	}
	for _, value := range []string{
		"1.2", "1.2.3-beta.1", "v1.2.3", "latest", "@scope/pkg@1.2.3", "1.2.3 ", "01.2.3",
	} {
		if _, err := ParseStableVersion(value); err == nil {
			t.Errorf("ParseStableVersion(%q) = nil error, want rejection", value)
		}
	}
}

func TestSortStableVersionsDeduplicatesDescending(t *testing.T) {
	got, err := SortStableVersions([]string{"1.2.0", "2.0.0", "1.10.0", "2.0.0", "1.2.0"})
	if err != nil {
		t.Fatalf("SortStableVersions: %v", err)
	}
	want := []string{"2.0.0", "1.10.0", "1.2.0"}
	if !slices.Equal(got, want) {
		t.Fatalf("sorted versions = %#v, want %#v", got, want)
	}
}

func TestBuildCatalogueFiltersBoundsAndRetainsExtras(t *testing.T) {
	published := make([]string, 0, 54)
	for minor := 0; minor < 52; minor++ {
		published = append(published, fmt.Sprintf("1.0.%d", minor))
	}
	published = append(published, "1.0.99-beta.1", "latest", "1.0.1")

	catalogue, err := BuildCatalogue(published, "1.0.50", "1.0.0")
	if err != nil {
		t.Fatalf("BuildCatalogue: %v", err)
	}
	if len(catalogue.Versions) != 51 {
		t.Fatalf("visible versions = %d, want 51", len(catalogue.Versions))
	}
	if catalogue.Versions[0].Version != "1.0.51" || catalogue.Versions[0].Latest {
		t.Fatalf("first option = %#v", catalogue.Versions[0])
	}
	if catalogue.Latest != "1.0.50" {
		t.Fatalf("latest = %q", catalogue.Latest)
	}
	if !catalogue.Has("1.0.0") || catalogue.Has("1.0.99-beta.1") || catalogue.Has("latest") {
		t.Fatal("catalogue target membership is incorrect")
	}
	latestFound := false
	for _, option := range catalogue.Versions {
		if option.Version == "1.0.50" {
			latestFound = option.Latest
		}
	}
	if !latestFound {
		t.Fatal("latest marker missing")
	}
}

func TestClassifyOperation(t *testing.T) {
	tests := []struct {
		name                    string
		active, current, target string
		want                    Operation
	}{
		{name: "update", active: "1.0.1", current: "1.0.1", target: "1.0.2", want: OperationUpdate},
		{name: "rollback", active: "1.0.3", current: "1.0.3", target: "1.0.2", want: OperationRollback},
		{name: "repair unknown", active: "", current: "", target: "1.0.2", want: OperationRepair},
		{name: "repair inactive current", active: "", current: "1.0.2", target: "1.0.2", want: OperationRepair},
		{name: "up to date", active: "1.0.2", current: "1.0.2", target: "1.0.2", want: OperationUpToDate},
		{name: "repair active drift", active: "1.0.3", current: "1.0.2", target: "1.0.3", want: OperationRepair},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ClassifyOperation(test.active, test.current, test.target)
			if err != nil {
				t.Fatalf("ClassifyOperation: %v", err)
			}
			if got != test.want {
				t.Fatalf("operation = %q, want %q", got, test.want)
			}
		})
	}
}
