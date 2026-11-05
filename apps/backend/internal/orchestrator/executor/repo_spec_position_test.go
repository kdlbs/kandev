package executor

import "testing"

func TestBuildRepoSpecsCarriesTheTaskPosition(t *testing.T) {
	specs := buildRepoSpecs([]*repoInfo{
		{TaskRepositoryID: "tr-1", RepositoryID: "r-1", Position: 0},
		{TaskRepositoryID: "tr-2", RepositoryID: "r-2", Position: 3},
	})
	if len(specs) != 2 || specs[0].Position == nil || *specs[0].Position != 0 || specs[1].Position == nil || *specs[1].Position != 3 {
		t.Fatalf("positions = %v, want 0 and 3", specs)
	}
}
