import { spawn, spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";

const tempDirs: string[] = [];

afterEach(() => {
  for (const dir of tempDirs.splice(0)) {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

function runGit(cwd: string, ...args: string[]): void {
  const result = spawnSync("git", args, { cwd, encoding: "utf8" });
  if (result.status !== 0) {
    throw new Error(`git ${args.join(" ")} failed: ${result.stderr}`);
  }
}

describe("E2E git shim", () => {
  it("holds a configured diff enrichment until released", async () => {
    const repoDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-git-shim-"));
    tempDirs.push(repoDir);
    const delayFile = path.join(repoDir, "delay");
    const startedFile = path.join(repoDir, "started");
    const releaseFile = path.join(repoDir, "release");
    runGit(repoDir, "init", "--initial-branch=main");
    fs.writeFileSync(
      delayFile,
      JSON.stringify({ subcommand: "diff", requiredArgs: ["--numstat"], startedFile, releaseFile }),
    );
    const child = spawn(
      process.execPath,
      [path.resolve("e2e/fixtures/git-shim.mjs"), "diff", "--numstat"],
      {
        cwd: repoDir,
        env: {
          ...process.env,
          KANDEV_E2E_GIT_DELAY_FILE: delayFile,
          KANDEV_E2E_ORIGINAL_PATH: process.env.PATH ?? "",
        },
      },
    );
    const completion = new Promise<number | null>((resolve, reject) => {
      child.on("error", reject);
      child.on("exit", resolve);
    });
    try {
      await expect.poll(() => fs.existsSync(startedFile), { timeout: 1000 }).toBe(true);
      expect(child.exitCode).toBe(null);
      fs.writeFileSync(releaseFile, "release");
      expect(await completion).toBe(0);
    } finally {
      child.kill();
    }
  });

  it.each([
    ["single global flag", ["--no-pager"]],
    ["two-token global option", ["-C", "."]],
    ["equals-form global option", ["--git-dir=.git"]],
  ])("intercepts push after a %s", (_name, globalArgs) => {
    const repoDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-git-shim-"));
    tempDirs.push(repoDir);
    const missingRemote = path.join(repoDir, "missing-remote.git");
    const recordFile = path.join(repoDir, "push-record");
    const expectedRemoteFile = path.join(repoDir, "expected-remote");
    fs.writeFileSync(expectedRemoteFile, missingRemote);
    runGit(repoDir, "init", "--initial-branch=main");
    runGit(repoDir, "remote", "add", "origin", missingRemote);

    const shim = path.resolve(process.cwd(), "e2e/fixtures/git-shim.mjs");
    const result = spawnSync(process.execPath, [shim, ...globalArgs, "push", "origin", "main"], {
      cwd: repoDir,
      env: {
        ...process.env,
        KANDEV_E2E_ORIGINAL_PATH: process.env.PATH ?? "",
        KANDEV_E2E_GITLAB_PUSH_FILE: expectedRemoteFile,
        KANDEV_E2E_GITLAB_PUSH_RECORD_FILE: recordFile,
      },
      encoding: "utf8",
    });

    expect(result.status, result.stderr).toBe(0);
    expect(fs.readFileSync(recordFile, "utf8")).toContain("push origin main");
  });
});
