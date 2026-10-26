package lifecycle

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/offlinebudget"
)

func TestOfflineBudgetFromMetadataAlwaysSendsAValue(t *testing.T) {
	cases := map[string]int{"": 15, "30": 30, "1440": 1440}
	cases["\t45\n"] = 45 // surrounding whitespace is ignored
	for in, want := range cases {
		md := map[string]interface{}{MetadataKeyOfflineBudgetMinutes: in}
		if got := offlineBudgetFromMetadata(md); got == nil || *got != want {
			t.Errorf("budget(%q) = %v, want %d", in, got, want)
		}
	}
	if got := offlineBudgetFromMetadata(nil); got == nil || *got != offlinebudget.DefaultMinutes {
		t.Errorf("budget(nil) = %v, want default", got)
	}
}

func TestValidateOfflineBudgetMetadataRejectsInvalid(t *testing.T) {
	for _, bad := range []string{"0", "1441", "-1", "1.5", "15m"} {
		err := validateOfflineBudgetMetadata(map[string]interface{}{MetadataKeyOfflineBudgetMinutes: bad})
		if !errors.Is(err, offlinebudget.ErrInvalid) {
			t.Errorf("validate(%q) = %v, want ErrInvalid", bad, err)
		}
	}
	if err := validateOfflineBudgetMetadata(nil); err != nil {
		t.Errorf("validate(nil) = %v", err)
	}
}
