import { spawnSync } from "node:child_process";
import { expect } from "@playwright/test";

export function dockerInspectExists(containerID: string): boolean {
  const res = spawnSync("docker", ["inspect", containerID], { stdio: "ignore" });
  return res.status === 0;
}

export function dockerRemove(containerID: string): void {
  spawnSync("docker", ["rm", "-f", containerID], { stdio: "ignore" });
}

export function dockerStop(containerID: string): void {
  const res = spawnSync("docker", ["stop", containerID], { stdio: "ignore" });
  if (res.status !== 0) {
    throw new Error(`failed to stop Docker container ${containerID}`);
  }
}

export function dockerState(containerID: string): string {
  const res = spawnSync("docker", ["inspect", "-f", "{{.State.Status}}", containerID], {
    encoding: "utf8",
  });
  if (res.status !== 0) return "missing";
  return res.stdout.trim();
}

export function dockerCurrentBranch(containerID: string): string {
  const res = spawnSync(
    "docker",
    ["exec", containerID, "git", "-C", "/workspace", "branch", "--show-current"],
    { encoding: "utf8" },
  );
  if (res.status !== 0) {
    const diag = spawnSync(
      "docker",
      [
        "exec",
        containerID,
        "sh",
        "-lc",
        "ls -la /workspace; git -C /workspace status --short --branch",
      ],
      { encoding: "utf8" },
    );
    const logs = spawnSync("docker", ["logs", "--tail", "40", containerID], { encoding: "utf8" });
    return [
      `ERR status=${res.status} state=${dockerState(containerID)}`,
      `stderr=${res.stderr.trim()}`,
      `diag=${diag.stdout.trim()} ${diag.stderr.trim()}`,
      `logs=${logs.stdout.trim()} ${logs.stderr.trim()}`,
    ].join("\n");
  }
  return res.stdout.trim();
}

export function dockerFileContent(containerID: string, filePath: string): string {
  const res = spawnSync("docker", ["exec", containerID, "cat", filePath], {
    encoding: "utf8",
  });
  if (res.status !== 0) {
    const logs = spawnSync("docker", ["logs", "--tail", "40", containerID], {
      encoding: "utf8",
    });
    throw new Error(
      [
        `failed to read ${filePath} from Docker container ${containerID}`,
        `stderr=${res.stderr.trim()}`,
        `logs=${logs.stdout.trim()} ${logs.stderr.trim()}`,
      ].join("\n"),
    );
  }
  return res.stdout;
}

export function dockerPathExists(containerID: string, filePath: string): boolean {
  return (
    spawnSync("docker", ["exec", containerID, "test", "-e", filePath], {
      stdio: "ignore",
    }).status === 0
  );
}

export async function waitForDockerContainerRemoved(
  containerID: string,
  message: string,
): Promise<void> {
  await expect
    .poll(() => dockerInspectExists(containerID), {
      timeout: 60_000,
      message,
    })
    .toBe(false);
}

export type DockerExecResult = { status: number | null; stdout: string; stderr: string };

export function dockerExec(containerID: string, ...command: string[]): DockerExecResult {
  const result = spawnSync("docker", ["exec", containerID, ...command], { encoding: "utf8" });
  return { status: result.status, stdout: result.stdout, stderr: result.stderr };
}

/**
 * Returns true when the container can perform a full user-namespace map,
 * which requires both the seccomp/AppArmor relaxation AND functioning
 * sub-id mapping (newuidmap/newgidmap setuid binaries or /etc/subuid
 * entries). Without this bwrap will fail with "setting up uid map:
 * Permission denied" even when namespace creation is allowed.
 *
 * The probe uses `unshare --map-root-user` which goes one step beyond
 * `unshare --user`: it also maps the UID inside the new namespace, the
 * operation bwrap needs. On hosts where user namespaces are allowed but
 * mapping is unavailable, `unshare --user` succeeds but this probe fails.
 */
export function dockerHasSubuidMapping(containerID: string): boolean {
  const result = spawnSync(
    "docker",
    ["exec", containerID, "sh", "-c", "unshare --map-root-user --user true 2>/dev/null"],
    { encoding: "utf8", stdio: "pipe" },
  );
  return result.status === 0;
}

export function dockerSecurityOpt(containerID: string): string[] | null {
  const result = spawnSync(
    "docker",
    ["inspect", "--format", "{{json .HostConfig.SecurityOpt}}", containerID],
    {
      encoding: "utf8",
    },
  );
  if (result.status !== 0)
    throw new Error(
      `failed to inspect Docker SecurityOpt for ${containerID}: ${result.stderr.trim()}`,
    );
  return JSON.parse(result.stdout) as string[] | null;
}

/** Creates a network with a non-default driver, reporting whether the host
 *  Docker accepted it. Some drivers are unavailable on some hosts. */
export function dockerNetworkCreateWithDriver(name: string, driver: string): boolean {
  return (
    spawnSync("docker", ["network", "create", "--driver", driver, name], { stdio: "ignore" })
      .status === 0
  );
}

export function dockerNetworkCreate(name: string): void {
  const result = spawnSync("docker", ["network", "create", name], { encoding: "utf8" });
  if (result.status !== 0) {
    throw new Error(`failed to create Docker network ${name}: ${result.stderr.trim()}`);
  }
}

/**
 * Removes a network, disconnecting anything still attached first.
 *
 * `docker network rm` refuses while a container holds an endpoint, and a
 * silent refusal leaves host state behind for every later run. This forces the
 * disconnect and then verifies the network is gone.
 */
export function dockerNetworkRemove(name: string): void {
  if (spawnSync("docker", ["network", "rm", name], { stdio: "ignore" }).status === 0) return;

  const inspect = spawnSync(
    "docker",
    ["network", "inspect", "--format", "{{json .Containers}}", name],
    { encoding: "utf8" },
  );
  // Already gone, or never created: nothing to clean up.
  if (inspect.status !== 0) return;

  for (const id of Object.keys(JSON.parse(inspect.stdout) as Record<string, unknown>)) {
    spawnSync("docker", ["network", "disconnect", "--force", name, id], { stdio: "ignore" });
  }
  spawnSync("docker", ["network", "rm", name], { stdio: "ignore" });

  const stillThere = spawnSync("docker", ["network", "inspect", name], { stdio: "ignore" });
  if (stillThere.status === 0) {
    throw new Error(`failed to remove Docker network ${name}; it is still on the host`);
  }
}

/** The names of every network a container is attached to. */
export function dockerContainerNetworks(containerID: string): string[] {
  const result = spawnSync(
    "docker",
    ["inspect", "--format", "{{json .NetworkSettings.Networks}}", containerID],
    { encoding: "utf8" },
  );
  if (result.status !== 0) {
    throw new Error(`failed to inspect networks for ${containerID}: ${result.stderr.trim()}`);
  }
  return Object.keys(JSON.parse(result.stdout) as Record<string, unknown>).sort();
}

/** The host port a container port is published on, or null when unpublished. */
export function dockerPublishedPort(containerID: string, containerPort: number): string | null {
  const result = spawnSync(
    "docker",
    ["inspect", "--format", `{{json (index .NetworkSettings.Ports "${containerPort}/tcp")}}`, containerID],
    { encoding: "utf8" },
  );
  if (result.status !== 0) {
    throw new Error(`failed to inspect published ports for ${containerID}: ${result.stderr.trim()}`);
  }
  const bindings = JSON.parse(result.stdout) as { HostPort?: string }[] | null;
  return bindings?.[0]?.HostPort ?? null;
}
