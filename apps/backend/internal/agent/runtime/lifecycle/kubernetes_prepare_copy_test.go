package lifecycle

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func installMetadataRestrictedCopy(t *testing.T, bin string) {
	t.Helper()
	realCopy, err := exec.LookPath("cp")
	if err != nil {
		t.Fatal(err)
	}
	wrapper := `#!/bin/sh
for arg in "$@"; do
  case "$arg" in
    -a|-p|--archive|--preserve|--preserve=*)
      echo 'cp: preserving metadata: Operation not permitted' >&2
      exit 1
      ;;
  esac
done
exec ` + shellQuote(realCopy) + ` "$@"
`
	if err := os.WriteFile(filepath.Join(bin, "cp"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
}
