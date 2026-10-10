import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";
import { describe, expect, it } from "vitest";
import { configureGitHubOrigin } from "./github-origin";

describe("configureGitHubOrigin", () => {
  it("publishes to the offline origin and restores shared checkout configuration", () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-github-origin-"));
    const remote = path.join(root, "origin.git");
    const checkout = path.join(root, "checkout");
    const environment = {
      ...process.env,
      GIT_CONFIG_NOSYSTEM: "1",
      GIT_CONFIG_GLOBAL: path.join(root, "global-config"),
    };
    const git = (cwd: string, ...args: string[]) =>
      execFileSync("git", args, { cwd, env: environment, encoding: "utf8" }).trim();

    try {
      git(root, "init", "--bare", "-b", "main", remote);
      git(root, "init", "-b", "main", checkout);
      git(checkout, "config", "user.name", "E2E Test");
      git(checkout, "config", "user.email", "e2e@test.local");
      git(checkout, "commit", "--allow-empty", "-m", "fixture snapshot");
      const originalOrigin = pathToFileURL(remote).href;
      const rewriteKey = `url.${originalOrigin}.insteadOf`;
      git(checkout, "remote", "add", "origin", originalOrigin);
      git(checkout, "config", "--add", rewriteKey, "https://github.com/other/repository.git");

      const githubURL = "https://github.com/test-owner/test-repo.git";
      const restore = configureGitHubOrigin(checkout, githubURL, environment);
      expect(git(checkout, "config", "--get", "remote.origin.url")).toBe(githubURL);
      git(checkout, "push", "origin", "HEAD:refs/pull/42/head");
      expect(git(root, "--git-dir", remote, "rev-parse", "refs/pull/42/head")).toBe(
        git(checkout, "rev-parse", "HEAD"),
      );

      restore();
      expect(git(checkout, "config", "--get", "remote.origin.url")).toBe(originalOrigin);
      expect(git(checkout, "config", "--get-all", rewriteKey)).toBe(
        "https://github.com/other/repository.git",
      );
    } finally {
      fs.rmSync(root, { recursive: true, force: true });
    }
  });
});
