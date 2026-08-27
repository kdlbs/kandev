import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { afterEach, describe, expect, it } from "vitest";

const scriptPath = path.resolve(__dirname, "run-e2e.sh");
const tempDirs: string[] = [];
const tempFiles: string[] = [];

afterEach(() => {
  for (const dir of tempDirs.splice(0)) fs.rmSync(dir, { recursive: true, force: true });
  for (const file of tempFiles.splice(0)) fs.rmSync(file, { force: true });
});

function runnerEnv(binDir: string, extra: Record<string, string> = {}): NodeJS.ProcessEnv {
  const env = {
    ...process.env,
    ...extra,
    PATH: `${binDir}:${process.env.PATH ?? ""}`,
  };
  delete env.KANDEV_E2E_CONTAINERS;
  delete env.KANDEV_E2E_DOCKER;
  delete env.CAPTURE_PR_ASSETS;
  return env;
}

describe("run-e2e.sh", () => {
  it("marks a managed containers run before invoking Playwright", () => {
    const binDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-runner-"));
    tempDirs.push(binDir);
    const pnpmPath = path.join(binDir, "pnpm");
    fs.writeFileSync(pnpmPath, "#!/usr/bin/env sh\nprintf '%s' \"${KANDEV_E2E_CONTAINERS:-}\"\n");
    fs.chmodSync(pnpmPath, 0o755);

    const result = spawnSync(
      "bash",
      [scriptPath, "--host", "--no-build", "--project", "containers", "--", "--help"],
      {
        encoding: "utf8",
        env: runnerEnv(binDir),
      },
    );

    expect(result.status).toBe(0);
    expect(result.stdout).toBe("1");
  });

  it("passes the marker to every host shard", () => {
    const binDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-runner-"));
    const resultFile = path.join(binDir, "marker.txt");
    tempDirs.push(binDir);
    tempFiles.push("/tmp/e2e-host-shard-1.log", "/tmp/e2e-host-shard-2.log");
    const pnpmPath = path.join(binDir, "pnpm");
    fs.writeFileSync(
      pnpmPath,
      '#!/usr/bin/env sh\nprintf \'%s\\n\' "${KANDEV_E2E_CONTAINERS:-}" >> "$KANDEV_RUNNER_RESULT_FILE"\n',
    );
    fs.chmodSync(pnpmPath, 0o755);

    const result = spawnSync(
      "bash",
      [
        scriptPath,
        "--host",
        "--no-build",
        "--shards",
        "2",
        "--project",
        "containers",
        "--",
        "--help",
      ],
      {
        encoding: "utf8",
        env: runnerEnv(binDir, { KANDEV_RUNNER_RESULT_FILE: resultFile }),
      },
    );

    expect(result.status).toBe(0);
    expect(fs.readFileSync(resultFile, "utf8").trim().split("\n")).toEqual(["1", "1"]);
  });

  it("passes the marker to the outer Docker runner", () => {
    const binDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-runner-"));
    const resultFile = path.join(binDir, "docker-args.txt");
    tempDirs.push(binDir);
    tempFiles.push("/tmp/e2e-docker-shard-1.log");
    const dockerPath = path.join(binDir, "docker");
    fs.writeFileSync(
      dockerPath,
      '#!/usr/bin/env sh\nif [ "$1" = "info" ]; then exit 1; fi\nif [ "$1" = "run" ]; then printf \'%s\\n\' "$*" >> "$KANDEV_RUNNER_RESULT_FILE"; exit 0; fi\nexit 1\n',
    );
    fs.chmodSync(dockerPath, 0o755);

    const result = spawnSync(
      "bash",
      [scriptPath, "--docker", "--no-build", "--project", "containers", "--", "--help"],
      {
        encoding: "utf8",
        env: runnerEnv(binDir, { KANDEV_RUNNER_RESULT_FILE: resultFile }),
      },
    );

    expect(result.status).toBe(0);
    const dockerInvocations = fs.readFileSync(resultFile, "utf8").trim().split("\n");
    expect(dockerInvocations.some((args) => args.includes("-e KANDEV_E2E_CONTAINERS=1"))).toBe(
      true,
    );
  });
});
