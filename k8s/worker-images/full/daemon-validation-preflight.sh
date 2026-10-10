# Embedded into the companion startup command by the opt-in template renderer.
export DOCKER_HOST=unix:///run/docker/docker.sock
receipt=/run/docker/validation-accounting.json
rm -f "$receipt"
probe=
daemon_pid=
phase=cgroup
cleanup_validation_daemon() {
  status=$?
  if [ "$status" -ne 0 ]; then echo "Validation companion preflight failed: stage=$phase status=$status" >&2; fi
  rm -f "$receipt"
  if [ -n "$probe" ]; then timeout 10 docker rm -f "$probe" >/dev/null 2>&1 || true; fi
  if [ -n "$daemon_pid" ]; then
    kill -TERM "$daemon_pid" 2>/dev/null || true
    wait "$daemon_pid" 2>/dev/null || true
  fi
}
trap cleanup_validation_daemon EXIT
trap 'exit 143' TERM
trap 'exit 130' INT
# Privileged CRI containers can see the host cgroup namespace. The mount root
# then is not this companion. Resolve our own group before creating descendants.
companion_group=$(awk -F: '$1 == "0" {print $3}' /proc/self/cgroup)
case "$companion_group" in /*) :;; *) exit 1;; esac
case "$companion_group" in *..*|*'//'*|*[!a-zA-Z0-9_./:-]*) exit 1;; esac
parent_group="/sys/fs/cgroup${companion_group%/}"
parent_limit=$(cat "$parent_group/memory.max")
if [ "$parent_limit" != "$validation_budget" ]; then
  echo "Validation companion budget mismatch: group=$companion_group limit=$parent_limit expected=$validation_budget" >&2
  exit 1
fi
# cgroup v2 domain controllers cannot be enabled while this group has processes.
# Move only this startup process, within its verified bounded companion group.
mkdir -p "$parent_group/init"
echo "$$" > "$parent_group/init/cgroup.procs"
echo '+cpu +memory +pids' > "$parent_group/cgroup.subtree_control"
validation_cgroup_parent="${companion_group%/}/docker"
phase=daemon
start_validation_daemon
ready_deadline=$(( $(date +%s) + 60 ))
until timeout 3 docker info >/dev/null 2>&1; do
  kill -0 "$daemon_pid"
  [ "$(date +%s)" -lt "$ready_deadline" ] || exit 1
  sleep 1
done
[ "$(timeout 3 docker info --format '{{.CgroupVersion}} {{.CgroupDriver}}')" = '2 cgroupfs' ]
phase=image
# Image availability is bounded separately from daemon readiness.
case "$validation_image" in
  sha256:*)
    # Disposable fixtures stream this exact image into the private daemon.
    image_deadline=$(( $(date +%s) + 180 ))
    until timeout 3 docker image inspect "$validation_image" >/dev/null 2>&1; do
      [ "$(date +%s)" -lt "$image_deadline" ] || exit 1
      sleep 1
    done;;
  *) timeout 3 docker image inspect "$validation_image" >/dev/null 2>&1 || timeout 180 docker pull "$validation_image" >/dev/null;;
esac
phase=probe
baseline=$(cat "$parent_group/memory.current")
probe=$(timeout 15 docker create --name "kandev-validation-probe-$(cat /proc/sys/kernel/random/uuid)" --pull=never \
  --memory=67108864 --memory-swap=67108864 --cpus=0.25 --pids-limit=32 \
  --user=1000:1000 --cap-drop=ALL --security-opt=no-new-privileges \
  --network=none --entrypoint=python3 "$validation_image" -c \
  'import time; data=bytearray(16*1024*1024); time.sleep(30)')
timeout 15 docker start "$probe" >/dev/null
pid=$(timeout 3 docker inspect --format '{{.State.Pid}}' "$probe")
case "$pid" in ''|*[!0-9]*) exit 1;; esac
[ "$pid" -gt 0 ]
child_group=$(awk -F: '$1 == "0" {print $3}' "/proc/$pid/cgroup")
case "$child_group" in *..*|*'//'*) exit 1;; esac
if [ "$child_group" != "$validation_cgroup_parent/$probe" ]; then
  echo 'Validation probe escaped companion cgroup' >&2
  exit 1
fi
child_limit=$(cat "/sys/fs/cgroup$child_group/memory.max")
[ "$child_limit" = 67108864 ]
probe_deadline=$(( $(date +%s) + 20 ))
while :; do
  child_current=$(cat "/sys/fs/cgroup$child_group/memory.current")
  parent_current=$(cat "$parent_group/memory.current")
  parent_delta=$(( parent_current - baseline ))
  if [ "$child_current" -ge 8388608 ] && [ "$parent_delta" -ge 8388608 ]; then break; fi
  [ "$(date +%s)" -lt "$probe_deadline" ] || exit 1
  sleep 1
done
daemon_id=$(timeout 3 docker info --format '{{.ID}}')
case "$daemon_id" in ''|*[!a-zA-Z0-9:_-]*) exit 1;; esac
generation=$(cat /proc/sys/kernel/random/uuid)
timeout 10 docker rm -f "$probe" >/dev/null
probe_id=$probe
probe=
# probe_cgroup is relative to the verified companion; retain its namespace path
# separately for diagnostics. Independent host acceptance checks actual ancestry.
printf '{"version":1,"daemon_id":"%s","generation":"%s","memory_max":%s,"probe_memory_max":%s,"probe_memory_current":%s,"probe_cgroup":"/docker/%s","companion_cgroup":"%s","parent_delta":%s}\n' \
  "$daemon_id" "$generation" "$parent_limit" "$child_limit" "$child_current" "$probe_id" "$companion_group" "$parent_delta" > "$receipt.tmp"
chmod 0644 "$receipt.tmp"
mv "$receipt.tmp" "$receipt"
phase=running
wait "$daemon_pid"
