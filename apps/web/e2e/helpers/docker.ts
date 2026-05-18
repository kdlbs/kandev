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
