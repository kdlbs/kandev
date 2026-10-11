#!/usr/bin/env bash
set -euo pipefail
isolated=false
if [[ ${1:-} == --isolated ]]; then isolated=true; shift; fi
image=${1:-}
[[ $# == 1 && "$image" =~ ^[a-z0-9][a-z0-9./:_-]*@sha256:[a-f0-9]{64}$ ]] || {
  echo 'Usage: render-template.sh [--isolated] registry/image@sha256:<64 lowercase hex digits>' >&2
  exit 2
}
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
python3 - "$here" "$image" "$isolated" <<'PY'
from pathlib import Path
import sys
root, image, isolated = Path(sys.argv[1]), sys.argv[2], sys.argv[3] == 'true'
source = (root/'pod-template.yaml').read_text().replace('FULL_WORKER_IMAGE_REQUIRED', image)
if isolated:
    host_line = "          - {name: DOCKER_HOST, value: 'unix:///run/docker/docker.sock'}"
    source = source.replace(host_line, host_line + "\n          - {name: FULL_WORKER_CHECK_MODE, value: 'isolated'}\n          - {name: FULL_WORKER_CHECK_IMAGE, value: '" + image + "'}")
    daemon_line = next(line for line in source.splitlines() if 'exec dockerd ' in line)
    helper = (root/'daemon-validation-preflight.sh').read_text()
    startup = '            validation_budget=3221225472\n'
    startup += "            validation_image='" + image + "'\n"
    startup += '            start_validation_daemon() {\n'
    startup += daemon_line.replace('exec dockerd ', '  dockerd ').replace('--cgroup-parent=docker', '--cgroup-parent="$validation_cgroup_parent"') + ' &\n'
    startup += '              daemon_pid=$!\n            }\n'
    startup += '\n'.join('            ' + line if line else '' for line in helper.splitlines())
    source = source.replace(daemon_line, startup)
print(source, end='')
PY
