package replay

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

const modulePath = "github.com/kandev/kandev/"

// allowedInternal are the only repository packages the evaluator may reach.
var allowedInternal = map[string]bool{
	modulePath + "internal/common/costs":            true,
	modulePath + "internal/coordinator/replay":      true,
	modulePath + "internal/coordinator/replay/stub": true,
}

func TestDependencyClosureIsAnAllowList(t *testing.T) {
	for _, pkg := range []string{"./", "./stub"} {
		out, err := exec.Command("go", "list", "-deps", pkg).CombinedOutput()
		if err != nil {
			t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
		}
		for _, dep := range strings.Fields(string(out)) {
			if strings.HasPrefix(dep, modulePath) && !allowedInternal[dep] {
				t.Errorf("%s depends on %s, which the allow-list does not name", pkg, dep)
			}
			if !strings.Contains(strings.Split(dep, "/")[0], ".") && dep != "" {
				continue // standard library
			}
			if !strings.HasPrefix(dep, modulePath) {
				t.Errorf("%s depends on third-party package %s", pkg, dep)
			}
		}
	}
}

func TestConstantsHaveTheirContractValues(t *testing.T) {
	if MinHeldOut != 20 || MinGainThousandths != 50 || CaseWindow != 100 || MaxOutputTokens != 4000 ||
		ReplayTimeout != 20*time.Minute || RunTimeout != 90*time.Second || Concurrency != 4 ||
		PriceTimeout != 5*time.Second || Attempts != 3 || MinAttemptsRan != 2 || FirstGroupSize != 50 ||
		SecondGroupSize != 50 || OverrideHorizon != 90*24*time.Hour || StaleAfter != 30*time.Minute ||
		Retention != 400*24*time.Hour {
		t.Fatal("a harness constant changed; the contract fixes these values")
	}
}
