import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { E2E_DOCKER_SCOPE } from "../fixtures/docker-probe";
import {
  generateKeypair,
  removeContainerIfExists,
  scanHostFingerprint,
  waitForTCPOpen,
  type SSHServerHandle,
} from "./ssh";

/**
 * Starts the SSH host that a remote Docker executor targets.
 *
 * Two differences from startSSHServer, both load-bearing:
 *
 *   - The machine's Docker socket is mounted in, so `docker system dial-stdio`
 *     over SSH reaches a real daemon. Nesting a second daemon would test a
 *     different thing and is far less stable.
 *   - The container shares the machine's network namespace. Task containers
 *     created through that socket publish agentctl on the *machine's*
 *     loopback, while the executor's port forward tunnels to this container's
 *     loopback. Without --network host those are different namespaces and the
 *     forward reaches nothing.
 *
 * --network host also means published-port mapping does not apply, so sshd
 * picks its own port through SSHD_PORT. It is scoped to this container: the
 * ordinary SSH specs keep their isolated namespace and their NET_ADMIN
 * fault injection.
 */
const REMOTE_DOCKER_BASE_PORT = 22500;

export function startRemoteDockerHost(
  workerIndex: number,
  imageTag: string,
  workDir: string,
): SSHServerHandle {
  fs.mkdirSync(workDir, { recursive: true });
  const port = REMOTE_DOCKER_BASE_PORT + workerIndex;
  const containerName = `kandev-rdocker-e2e-${E2E_DOCKER_SCOPE}-${workerIndex}`;

  const identityFile = path.join(workDir, "id_ed25519");
  const publicKeyFile = `${identityFile}.pub`;
  generateKeypair(identityFile);

  removeContainerIfExists(containerName);
  // Mounted at the identical absolute path inside the container so that a
  // path the SSH host reports resolves to the same directory when the daemon
  // uses it as a mount source. See the entrypoint's KANDEV_REMOTE_HOME block.
  const remoteHome = path.join(workDir, "remote-home");
  fs.mkdirSync(remoteHome, { recursive: true });

  const containerId = execFileSync(
    "docker",
    [
      "run",
      "-d",
      "--rm",
      "--name",
      containerName,
      // Deliberately NOT labelled kandev.e2e.run: the per-test container
      // sweep removes everything carrying that scope, and this host must
      // outlive it. The worker fixture owns its lifetime through
      // stopSSHServer. The scoped label below still identifies strays for an
      // external sweeper without matching that filter.
      "--label",
      `kandev.e2e.rdocker-host=${E2E_DOCKER_SCOPE}`,
      "--label",
      "kandev.e2e.role=remote-docker-target",
      "--network",
      "host",
      "-e",
      `SSHD_PORT=${port}`,
      "-v",
      "/var/run/docker.sock:/var/run/docker.sock",
      "-v",
      `${publicKeyFile}:/authorized_keys:ro`,
      "-e",
      `KANDEV_REMOTE_HOME=${remoteHome}`,
      "-v",
      `${remoteHome}:${remoteHome}`,
      imageTag,
    ],
    { encoding: "utf8" },
  )
    .toString()
    .trim();

  waitForTCPOpen("127.0.0.1", port);
  const hostFingerprint = scanHostFingerprint("127.0.0.1", port);

  return {
    containerId,
    containerName,
    host: "127.0.0.1",
    port,
    user: "kandev",
    identityFile,
    publicKeyFile,
    hostFingerprint,
    workDir,
  };
}
