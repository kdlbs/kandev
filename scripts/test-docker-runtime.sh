#!/usr/bin/env bash
set -euo pipefail

if (($# == 0)); then
  echo 'Usage: scripts/test-docker-runtime.sh IMAGE [IMAGE ...]' >&2
  exit 2
fi

container=
cleanup() {
  if [[ -n "$container" ]]; then
    docker rm --force --volumes "$container" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

for image in "$@"; do
  image_id=$(docker image inspect --format '{{.Id}}' "$image")
  container="kandev-runtime-smoke-${BASHPID}-${RANDOM}"
  # Use the image's entrypoint so the check includes its normal privilege drop.
  timeout 90 docker run --rm --interactive --network=none --name "$container" \
    "$image" python3 - <<'PY'
import os
import signal
import subprocess
import tempfile


# @covers AC-EXECUTORS-CONTAINER-TOOLS-001.1, AC-EXECUTORS-CONTAINER-TOOLS-001.3
assert os.getuid() == 1000, f"expected runtime UID 1000, got {os.getuid()}"
probe = r'''
parent_pid=$(ps -o ppid= -p "$$") || exit $?
parent_pid=${parent_pid//[[:space:]]/}
[[ "$parent_pid" == "$1" ]] || exit 1
kill -0 "$1"
'''


def run(command, expected_exit=0):
    child = subprocess.Popen(
        ["/bin/bash", "-c", command, "runtime-smoke", str(os.getpid())],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        start_new_session=True,
    )
    try:
        stdout, stderr = child.communicate(timeout=10)
    finally:
        try:
            os.killpg(child.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        child.wait()
    assert child.returncode == expected_exit, (
        f"command {command!r}: expected exit {expected_exit}, "
        f"got {child.returncode}; stdout={stdout!r}; stderr={stderr!r}"
    )
    return stdout, stderr


assert run(probe) == ("", ""), "unexpected parent-probe output"

# @covers AC-EXECUTORS-CONTAINER-TOOLS-001.2
# These are image prerequisites and shell checks, not an authenticated Droid run.
with tempfile.TemporaryDirectory(prefix="kandev-runtime-smoke-") as workspace:
    os.chdir(workspace)
    with open("runtime-smoke-marker", "w") as marker:
        marker.write("runtime smoke\n")
    for _ in range(20):
        stdout, stderr = run("pwd")
        assert stdout == workspace + "\n" and stderr == "", (stdout, stderr)
        stdout, stderr = run("ls -la")
        assert "runtime-smoke-marker" in stdout and stderr == "", (stdout, stderr)
        assert run("cd /tmp") == ("", "")
        assert run("printf 'execute-test\\n'") == ("execute-test\n", "")
        stdout, stderr = run("date")
        assert stdout.strip() and stderr == "", (stdout, stderr)
        stdout, stderr = run("id")
        assert "uid=1000(" in stdout and stderr == "", (stdout, stderr)
        stdout, stderr = run("uname -srm")
        assert stdout.startswith("Linux ") and stderr == "", (stdout, stderr)

    # Stay alive across the supervisor's two-second polling interval.
    assert run(probe + "\nsleep 3\n" + probe) == ("", "")
    assert run("printf 'expected-stderr\\n' >&2; exit 7", 7) == (
        "", "expected-stderr\n"
    )
    os.chdir("/tmp")

print("UID 1000: parent probe, 140 ordered shell commands, delayed probe and exit status passed")
PY
  container=
  echo "runtime image smoke passed: $image ($image_id)"
done
