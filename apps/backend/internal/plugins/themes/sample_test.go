package themes

import (
	"os"
	"testing"
)

func TestSharedSamplePalettes(t *testing.T) {
	for _, v := range []struct{ file, base string }{{"night", "dark"}, {"paper", "light"}} {
		data, err := os.ReadFile("testdata/sample/themes/" + v.file + ".json")
		if err != nil {
			t.Fatal(err)
		}
		d, err := Parse(data, v.base)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Digest()) != 64 {
			t.Fatal("invalid digest")
		}
	}
}
