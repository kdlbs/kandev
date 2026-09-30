import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { describe, expect, it, vi } from "vitest";

vi.mock("../pages/kanban-page", () => ({ KanbanPage: vi.fn() }));
vi.mock("../pages/session-page", () => ({ SessionPage: vi.fn() }));
import { GitHelper, makeGitEnv } from "./git-helper";

describe("GitHelper.pushMainWithRetry", () => {
  it("does not rebase or retry a server-side rejection", () => {
    const helper = new GitHelper("unused", {});
    const rejection = new Error("remote rejected: protected branch hook declined");
    const exec = vi.spyOn(helper, "exec").mockImplementation(() => {
      throw rejection;
    });
    expect(() => helper.pushMainWithRetry()).toThrow(rejection);
    expect(exec.mock.calls).toEqual([["git push origin main"]]);
  });
  it("rebases a local fixture commit when origin/main advanced", () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-git-helper-"));
    const remote = path.join(root, "remote.git");
    const working = path.join(root, "working");
    const concurrent = path.join(root, "concurrent");
    const env = makeGitEnv(root);
    const git = (cwd: string, ...args: string[]) => {
      execFileSync("git", args, { cwd, env, stdio: "pipe" });
    };

    try {
      fs.mkdirSync(working);
      git(root, "init", "--bare", "--initial-branch=main", remote);
      git(working, "init", "--initial-branch=main");
      git(working, "config", "user.name", "E2E Test");
      git(working, "config", "user.email", "e2e@test.local");
      fs.writeFileSync(path.join(working, "base.txt"), "base\n");
      git(working, "add", "base.txt");
      git(working, "commit", "-m", "seed fixture");
      git(working, "remote", "add", "origin", `file://${remote}`);
      git(working, "push", "origin", "main");

      git(root, "clone", `file://${remote}`, concurrent);
      git(concurrent, "config", "user.name", "E2E Test");
      git(concurrent, "config", "user.email", "e2e@test.local");
      fs.writeFileSync(path.join(concurrent, "remote.txt"), "remote update\n");
      git(concurrent, "add", "remote.txt");
      git(concurrent, "commit", "-m", "advance fixture origin");
      git(concurrent, "push", "origin", "main");

      fs.writeFileSync(path.join(working, "local.txt"), "local update\n");
      git(working, "add", "local.txt");
      git(working, "commit", "-m", "add local fixture");
      new GitHelper(working, env).pushMainWithRetry();

      const localHead = execFileSync("git", ["rev-parse", "HEAD"], {
        cwd: working,
        env,
        encoding: "utf8",
      }).trim();
      const remoteHead = execFileSync(
        "git",
        ["--git-dir", remote, "rev-parse", "refs/heads/main"],
        {
          env,
          encoding: "utf8",
        },
      ).trim();
      expect(remoteHead).toBe(localHead);
      expect(fs.readFileSync(path.join(working, "remote.txt"), "utf8")).toBe("remote update\n");
      expect(fs.readFileSync(path.join(working, "local.txt"), "utf8")).toBe("local update\n");
    } finally {
      fs.rmSync(root, { recursive: true, force: true });
    }
  });
});
