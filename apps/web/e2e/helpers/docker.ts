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

// dockerRestartDiagnostics captures the state that is lost when a restarted
// container exits before the session launch request returns. Keep this limited
// to non-secret Docker inspect fields so failed E2E artifacts are actionable
// without exposing the container environment.
export function dockerRestartDiagnostics(containerID: string): string {
  const inspect = spawnSync(
    "docker",
    [
      "inspect",
      "--format",
      [
        "state={{json .State}}",
        "entrypoint={{json .Config.Entrypoint}}",
        "command={{json .Config.Cmd}}",
        "mounts={{json .Mounts}}",
      ].join("\\n"),
      containerID,
    ],
    { encoding: "utf8" },
  );
  const logs = spawnSync("docker", ["logs", "--timestamps", containerID], { encoding: "utf8" });

  return [
    `container_id=${containerID}`,
    `inspect_status=${inspect.status}`,
    `inspect_stdout=${inspect.stdout.trim()}`,
    `inspect_stderr=${inspect.stderr.trim()}`,
    `logs_status=${logs.status}`,
    `logs_stdout=${logs.stdout.trim()}`,
    `logs_stderr=${logs.stderr.trim()}`,
  ].join("\\n");
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
