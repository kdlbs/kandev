#!/usr/bin/env bash
# Builds the sshd host used by the remote Docker integration test.
#
# The host carries the Docker CLI and is run with the machine's own daemon
# socket mounted in, so `docker system dial-stdio` over SSH reaches a real
# daemon. That tests the transport honestly without nesting a second daemon.
#
# Usage:
#   bash scripts/build-remote-docker-test-image.sh
#   KANDEV_TEST_REMOTE_DOCKER=1 go test ./internal/agent/runtime/lifecycle/ \
#     -run TestRemoteDockerTransportReachesARealDaemon -count=1
set -euo pipefail

IMAGE_TAG="${IMAGE_TAG:-kandev-rdocker-int:test}"
CTX="$(mktemp -d)"
trap 'rm -rf "$CTX"' EXIT

cat > "$CTX/Dockerfile" <<'EOF'
FROM alpine:3.20
# shadow supplies usermod; Alpine's adduser -D leaves the account locked and
# sshd then refuses every auth method, including public keys.
RUN apk add --no-cache openssh-server openssh-sftp-server docker-cli bash coreutils shadow \
 && printf "\nAllowTcpForwarding yes\nMaxSessions 100\n" > /etc/ssh/sshd_config.d/10-kandev.conf \
 && adduser -D -s /bin/bash kandev \
 && usermod -p '*' kandev \
 && mkdir -p /home/kandev/.ssh /var/empty /var/run/sshd \
 && chown -R kandev:kandev /home/kandev
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
EXPOSE 22
ENTRYPOINT ["/entrypoint.sh"]
EOF

cat > "$CTX/entrypoint.sh" <<'EOF'
#!/bin/sh
set -e
[ -f /etc/ssh/ssh_host_ed25519_key ] || ssh-keygen -A >/dev/null
if [ -n "$AUTHORIZED_KEY" ]; then
  echo "$AUTHORIZED_KEY" > /home/kandev/.ssh/authorized_keys
  chown -R kandev:kandev /home/kandev/.ssh
  chmod 700 /home/kandev/.ssh
  chmod 600 /home/kandev/.ssh/authorized_keys
fi
# The mounted socket keeps the host's ownership, so the SSH user needs a group
# with that GID to read it. Without this the daemon step fails with a
# permission error, which is a real cause but not the one under test.
if [ -S /var/run/docker.sock ]; then
  SOCK_GID="$(stat -c %g /var/run/docker.sock)"
  addgroup -g "$SOCK_GID" dockersock 2>/dev/null || true
  addgroup kandev "$(getent group "$SOCK_GID" | cut -d: -f1)" 2>/dev/null || true
fi
exec /usr/sbin/sshd -D -e
EOF

docker build -q -t "$IMAGE_TAG" "$CTX"
echo "built $IMAGE_TAG"
