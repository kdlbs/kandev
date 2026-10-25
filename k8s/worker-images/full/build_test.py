"""Fast process-boundary checks; these do not establish Docker acceptance."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


class BuildCleanupTest(unittest.TestCase):
    def test_verification_workspace_allows_built_binaries_to_execute(self):
        script = (Path(__file__).resolve().parent / "build.sh").read_text()
        self.assertIn(
            "--tmpfs /workspace:rw,exec,uid=1000,gid=1000,size=2g",
            script,
        )

    def test_failed_verification_reclaims_its_container(self):
        recipe = Path(__file__).resolve().parent
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            docker = root / "docker"
            docker.write_text('''#!/bin/sh
set -eu
case "$1 $2" in
  'image inspect')
    case "$*" in *--format*) printf 'sha256:fixture-image\\n';; *) exit 1;; esac;;
  'image rm') :;;
  'buildx create'|'buildx inspect'|'buildx build'|'buildx rm') :;;
  'inspect --format') printf '4294967296 4294967296 100000 200000\\n';;
  'run --rm')
    name=unnamed
    while [ "$#" -gt 0 ]; do
      if [ "$1" = --name ]; then shift; name=$1; fi
      shift
    done
    printf '%s' "$name" > "$CONTAINER_STATE"
    exit 1;;
  'rm --force')
    test "$(cat "$CONTAINER_STATE")" = "$3"
    rm "$CONTAINER_STATE";;
  *) echo "Unexpected Docker operation: $*" >&2; exit 2;;
esac
''')
            docker.chmod(0o755)
            df = root / "df"
            df.write_text("#!/bin/sh\nprintf 'Filesystem 1024-blocks Used Available Capacity Mounted\\nfixture 99999999 0 99999999 0%% /\\n'\n")
            df.chmod(0o755)
            state = root / "container-state"
            result = subprocess.run(
                ["bash", str(recipe / "build.sh"), "--build", "--verify"],
                env={**os.environ, "PATH": f"{root}:{os.environ['PATH']}",
                     "CONTAINER_STATE": str(state)},
                capture_output=True, text=True, timeout=10,
            )
            self.assertNotEqual(result.returncode, 0, result.stdout)
            self.assertFalse(state.exists(), "failed verifier left its container alive")


if __name__ == "__main__":
    unittest.main()
