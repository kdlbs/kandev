import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { describe, expect, it, vi } from "vitest";

vi.mock("../pages/kanban-page", () => ({ KanbanPage: vi.fn() }));
vi.mock("../pages/session-page", () => ({ SessionPage: vi.fn() }));
import type { ApiClient } from "./api-client";
import { GitHelper, makeGitEnv, createStandardProfile } from "./git-helper";

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

describe("createStandardProfile", () => {
  it("creates a mock profile when disabled and virtual agents precede the mock agent", async () => {
    const createAgentProfile = vi.fn().mockResolvedValue({ id: "profile-id" });
    const apiClient = {
      listAgents: vi.fn().mockResolvedValue({
        agents: [
          { id: "disabled-id", name: "openai-compatible" },
          { id: "dynamic", name: "dynamic" },
          { id: "mock-id", name: "mock-agent" },
        ],
      }),
      createAgentProfile,
    } as unknown as ApiClient;

    await expect(createStandardProfile(apiClient, "file-viewer")).resolves.toEqual({
      id: "profile-id",
    });
    expect(createAgentProfile).toHaveBeenCalledWith("mock-id", "file-viewer", {
      model: "mock-fast",
      auto_approve: true,
      cli_passthrough: false,
    });
  });

  it("reports a missing mock agent instead of creating a profile for another family", async () => {
    const createAgentProfile = vi.fn();
    const apiClient = {
      listAgents: vi.fn().mockResolvedValue({
        agents: [{ id: "dynamic", name: "dynamic" }],
      }),
      createAgentProfile,
    } as unknown as ApiClient;

    await expect(createStandardProfile(apiClient, "file-viewer")).rejects.toThrow(
      "mock-agent unavailable in test fixtures",
    );
    expect(createAgentProfile).not.toHaveBeenCalled();
  });
});
