import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { afterEach, describe, expect, it } from "vitest";

const scriptPath = path.resolve(__dirname, "run-e2e.sh");
const rawScriptPath = path.resolve(__dirname, "run-raw-e2e.sh");
const tempDirs: string[] = [];
const tempFiles: string[] = [];

afterEach(() => {
  for (const dir of tempDirs.splice(0)) fs.rmSync(dir, { recursive: true, force: true });
  for (const file of tempFiles.splice(0)) fs.rmSync(file, { force: true });
});

function runnerEnv(binDir: string, extra: Record<string, string> = {}): NodeJS.ProcessEnv {
  const env = {
    ...process.env,
    KANDEV_E2E_ALLOW_UNSAFE_PARALLELISM: "",
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

  it("marks a managed Kubernetes compatibility run before invoking Playwright", () => {
    const binDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-runner-"));
    tempDirs.push(binDir);
    const pnpmPath = path.join(binDir, "pnpm");
    fs.writeFileSync(pnpmPath, "#!/usr/bin/env sh\nprintf '%s' \"${KANDEV_E2E_CONTAINERS:-}\"\n");
    fs.chmodSync(pnpmPath, 0o755);

    const result = spawnSync(
      "bash",
      [scriptPath, "--host", "--no-build", "--project", "kubernetes-compat", "--", "--help"],
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
        env: runnerEnv(binDir, {
          KANDEV_RUNNER_RESULT_FILE: resultFile,
          KANDEV_E2E_ALLOW_UNSAFE_PARALLELISM: "1",
        }),
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

  it("treats the deprecated docker project name as an alias for containers", () => {
    const binDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-runner-"));
    tempDirs.push(binDir);
    const pnpmPath = path.join(binDir, "pnpm");
    fs.writeFileSync(pnpmPath, "#!/usr/bin/env sh\nprintf '%s' \"${KANDEV_E2E_CONTAINERS:-}\"\n");
    fs.chmodSync(pnpmPath, 0o755);

    const result = spawnSync(
      "bash",
      [scriptPath, "--host", "--no-build", "--project", "docker", "--", "--help"],
      {
        encoding: "utf8",
        env: runnerEnv(binDir),
      },
    );

    expect(result.status).toBe(0);
    expect(result.stdout).toBe("1");
  });

  it("accepts the deprecated docker project alias with equals syntax", () => {
    const binDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-runner-"));
    tempDirs.push(binDir);
    const pnpmPath = path.join(binDir, "pnpm");
    fs.writeFileSync(pnpmPath, "#!/usr/bin/env sh\nprintf '%s' \"${KANDEV_E2E_CONTAINERS:-}\"\n");
    fs.chmodSync(pnpmPath, 0o755);

    const result = spawnSync(
      "bash",
      [scriptPath, "--host", "--no-build", "--project=docker", "--", "--help"],
      {
        encoding: "utf8",
        env: runnerEnv(binDir),
      },
    );

    expect(result.status).toBe(0);
    expect(result.stdout).toBe("1");
  });

  it("normalizes the deprecated docker project alias for raw Playwright runs", () => {
    const binDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-raw-"));
    tempDirs.push(binDir);
    const pnpmPath = path.join(binDir, "pnpm");
    fs.writeFileSync(pnpmPath, "#!/usr/bin/env sh\nprintf '%s' \"$*\"\n");
    fs.chmodSync(pnpmPath, 0o755);

    const result = spawnSync("bash", [rawScriptPath, "--project=docker", "--help"], {
      encoding: "utf8",
      env: {
        ...process.env,
        KANDEV_E2E_ALLOW_UNSAFE_PARALLELISM: "",
        PATH: `${binDir}:${process.env.PATH ?? ""}`,
      },
    });

    expect(result.status).toBe(0);
    expect(result.stdout).toBe(
      "exec playwright test --config e2e/playwright.config.ts --workers=1 --project=containers --help",
    );
  });

  it("clears inherited container flags before an ordinary managed run", () => {
    const binDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-runner-"));
    tempDirs.push(binDir);
    const pnpmPath = path.join(binDir, "pnpm");
    fs.writeFileSync(
      pnpmPath,
      '#!/usr/bin/env sh\nprintf \'CONTAINERS=%s DOCKER=%s\' "${KANDEV_E2E_CONTAINERS:-unset}" "${KANDEV_E2E_DOCKER:-unset}"\n',
    );
    fs.chmodSync(pnpmPath, 0o755);

    const result = spawnSync(
      "bash",
      [scriptPath, "--host", "--no-build", "--project", "chromium", "--", "--help"],
      {
        encoding: "utf8",
        env: {
          ...process.env,
          PATH: `${binDir}:${process.env.PATH ?? ""}`,
          KANDEV_E2E_CONTAINERS: "1",
          KANDEV_E2E_DOCKER: "1",
        },
      },
    );

    expect(result.status).toBe(0);
    expect(result.stdout).toBe("CONTAINERS=unset DOCKER=unset");
  });

  it("builds the Linux helper targets for the deprecated docker project", () => {
    const binDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-runner-"));
    const makeLogFile = path.join(binDir, "make.log");
    const pnpmLogFile = path.join(binDir, "pnpm.log");
    tempDirs.push(binDir);
    tempFiles.push("/tmp/e2e-host-shard-1.log");

    const makePath = path.join(binDir, "make");
    fs.writeFileSync(
      makePath,
      '#!/usr/bin/env sh\nprintf \'%s\\n\' "$*" >> "$KANDEV_RUNNER_MAKE_LOG"\nexit 0\n',
    );
    fs.chmodSync(makePath, 0o755);

    const pnpmPath = path.join(binDir, "pnpm");
    fs.writeFileSync(
      pnpmPath,
      '#!/usr/bin/env sh\nprintf \'%s\\n\' "$*" >> "$KANDEV_RUNNER_PNPM_LOG"\nexit 0\n',
    );
    fs.chmodSync(pnpmPath, 0o755);

    const result = spawnSync(
      "bash",
      [scriptPath, "--host", "--project", "docker", "--", "--help"],
      {
        encoding: "utf8",
        env: runnerEnv(binDir, {
          KANDEV_RUNNER_MAKE_LOG: makeLogFile,
          KANDEV_RUNNER_PNPM_LOG: pnpmLogFile,
        }),
      },
    );

    expect(result.status).toBe(0);
    const makeInvocations = fs.readFileSync(makeLogFile, "utf8").trim().split("\n");
    expect(
      makeInvocations.some(
        (args) => args.includes("build-agentctl-linux") && args.includes("build-mock-agent-linux"),
      ),
    ).toBe(true);
  });

  it("rejects more than the local shard budget without an explicit opt-in", () => {
    const binDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-runner-"));
    tempDirs.push(binDir);
    const pnpmPath = path.join(binDir, "pnpm");
    fs.writeFileSync(pnpmPath, "#!/usr/bin/env sh\nexit 0\n");
    fs.chmodSync(pnpmPath, 0o755);

    const result = spawnSync(
      "bash",
      [
        scriptPath,
        "--host",
        "--no-build",
        "--shards",
        "4",
        "--project",
        "chromium",
        "--",
        "--help",
      ],
      {
        encoding: "utf8",
        env: runnerEnv(binDir),
      },
    );

    expect(result.status).toBe(1);
    expect(`${result.stdout}\n${result.stderr}`).toContain("local shard limit");

    const allowed = spawnSync(
      "bash",
      [
        scriptPath,
        "--host",
        "--no-build",
        "--shards",
        "4",
        "--project",
        "chromium",
        "--",
        "--help",
      ],
      {
        encoding: "utf8",
        env: runnerEnv(binDir, { KANDEV_E2E_ALLOW_UNSAFE_PARALLELISM: "1" }),
      },
    );

    expect(allowed.status).toBe(0);
  });

  it("rejects Playwright worker overrides above one", () => {
    const binDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-runner-"));
    tempDirs.push(binDir);
    const pnpmPath = path.join(binDir, "pnpm");
    fs.writeFileSync(pnpmPath, "#!/usr/bin/env sh\nexit 0\n");
    fs.chmodSync(pnpmPath, 0o755);

    const result = spawnSync(
      "bash",
      [scriptPath, "--host", "--no-build", "--project", "chromium", "--", "--workers=2"],
      {
        encoding: "utf8",
        env: runnerEnv(binDir),
      },
    );

    expect(result.status).toBe(1);
    expect(`${result.stdout}\n${result.stderr}`).toContain("Playwright worker limit");
  });

  it("applies the worker guard to raw Playwright runs", () => {
    const binDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-raw-"));
    tempDirs.push(binDir);
    const playwrightPath = path.join(binDir, "playwright");
    fs.writeFileSync(playwrightPath, "#!/usr/bin/env sh\nexit 0\n");
    fs.chmodSync(playwrightPath, 0o755);

    const result = spawnSync("bash", [rawScriptPath, "--workers=2", "--help"], {
      encoding: "utf8",
      env: {
        ...process.env,
        KANDEV_E2E_ALLOW_UNSAFE_PARALLELISM: "",
        PATH: `${binDir}:${process.env.PATH ?? ""}`,
      },
    });

    expect(result.status).toBe(2);
    expect(`${result.stdout}\n${result.stderr}`).toContain("Playwright worker limit");
  });
});
