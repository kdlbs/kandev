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
 * Returns true when the container has configured UID/GID sub-id ranges,
 * without which bwrap (and any other tool using user namespaces) will fail
 * with "setting up uid map: Permission denied" even when the seccomp and
 * AppArmor layers allow the namespace creation.
 */
export function dockerHasSubuidMapping(containerID: string): boolean {
  const result = spawnSync(
    "docker",
    ["exec", containerID, "sh", "-c", "head -1 /etc/subuid /etc/subgid 2>/dev/null | grep -q ."],
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
