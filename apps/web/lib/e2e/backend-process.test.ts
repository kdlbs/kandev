import type { ChildProcess } from "node:child_process";
import { describe, expect, test, vi } from "vitest";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { runOwnedBackendFixture, writeBackendStartupArtifact } from "../../e2e/fixtures/backend";
import { killProcessGroup } from "../../e2e/fixtures/process-group";

function childProcessWithPid(pid: number): ChildProcess {
  return { pid } as ChildProcess;
}

describe("killProcessGroup", () => {
  test("uses taskkill for the full process tree on Windows", async () => {
    const killWindowsTree = vi.fn((_pid: number, done: (error?: Error) => void) => done());

    await killProcessGroup(childProcessWithPid(42), "win32", killWindowsTree);

    expect(killWindowsTree).toHaveBeenCalledOnce();
    expect(killWindowsTree).toHaveBeenCalledWith(42, expect.any(Function));
  });

  test("reports a taskkill failure while the process is still alive", async () => {
    const failure = new Error("taskkill failed");
    const killWindowsTree = (_pid: number, done: (error?: Error) => void) => done(failure);

    await expect(
      killProcessGroup(childProcessWithPid(42), "win32", killWindowsTree, () => true),
    ).rejects.toBe(failure);
  });

  test("accepts a taskkill race when the process has already exited", async () => {
    const killWindowsTree = (_pid: number, done: (error?: Error) => void) =>
      done(new Error("not found"));

    await expect(
      killProcessGroup(childProcessWithPid(42), "win32", killWindowsTree, () => false),
    ).resolves.toBeUndefined();
  });
});

describe("backend startup diagnostic", () => {
  test("retains a bounded sanitized artifact after cleanup without replacing the startup failure", async () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-backend-diagnostic-"));
    const ownedDir = path.join(root, "owned");
    const outputDir = path.join(root, "output");
    fs.mkdirSync(ownedDir);
    const processLogPath = path.join(ownedDir, "backend-process.log");
    const backendLogPath = path.join(ownedDir, "backend.log");
    fs.writeFileSync(
      processLogPath,
      `TOKEN=secret-token /tmp/private/process\n${"x".repeat(70 * 1024)}`,
    );
    fs.writeFileSync(backendLogPath, "Authorization: Bearer private-value /data/tasks/secret\n");
    const startupFailure = new Error("startup TOKEN=error-secret /home/private");
    let locator = "";

    try {
      await expect(
        runOwnedBackendFixture(
          ownedDir,
          async () => {
            throw startupFailure;
          },
          {
            onFailure: (failure) => {
              expect(failure).toBe(startupFailure);
              locator = writeBackendStartupArtifact({
                outputDir,
                projectName: "mobile chrome",
                shard: "2/3",
                workerIndex: 4,
                parallelIndex: 1,
                port: 18099,
                startedAt: new Date("2026-09-19T16:00:00.000Z"),
                elapsedMs: 17,
                exitCode: 1,
                signal: null,
                processLogPath,
                backendLogPath,
                failure,
              });
            },
          },
        ),
      ).rejects.toBe(startupFailure);

      const artifactPath = path.join(outputDir, locator);
      const artifact = fs.readFileSync(artifactPath, "utf8");
      expect(locator).toMatch(/^worker-backend\/mobile-chrome-shard-2-3-worker-4-parallel-1-/);
      expect(Buffer.byteLength(artifact)).toBeLessThanOrEqual(64 * 1024);
      expect(artifact).toContain("utc=2026-09-19T16:00:00.000Z");
      expect(artifact).toContain("elapsed_ms=17");
      expect(artifact).toContain("exit_code=1");
      expect(artifact).not.toContain("secret-token");
      expect(artifact).not.toContain("private-value");
      expect(artifact).not.toContain("error-secret");
      expect(artifact).not.toContain("/tmp/private");
      expect(artifact).not.toContain("/data/tasks");
      expect(artifact).not.toContain("/home/private");
      if (process.platform !== "win32") expect(fs.statSync(artifactPath).mode & 0o777).toBe(0o600);
      expect(fs.existsSync(ownedDir)).toBe(false);
    } finally {
      fs.rmSync(root, { recursive: true, force: true });
    }
  });

  test("does not create an artifact after successful fixture use", async () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-backend-diagnostic-success-"));
    const ownedDir = path.join(root, "owned");
    fs.mkdirSync(ownedDir);
    const onFailure = vi.fn();

    try {
      await runOwnedBackendFixture(ownedDir, async () => {}, { onFailure });
      expect(onFailure).not.toHaveBeenCalled();
      expect(fs.existsSync(ownedDir)).toBe(false);
    } finally {
      fs.rmSync(root, { recursive: true, force: true });
    }
  });
});
