import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { restoreSeedRepositoryOrigin } from "../fixtures/seed-repository-origin";

function withSeedRepository(
  run: (seed: { repositoryPath: string; repositoryRemoteURL: string }) => void,
) {
  const root = mkdtempSync(path.join(tmpdir(), "kandev-seed-origin-unit-"));
  const repositoryPath = path.join(root, "checkout");
  const repositoryRemoteURL = path.join(root, "remote.git");
  const git = (args: string[]) => execFileSync("git", args, { stdio: "pipe" });
  try {
    git(["init", "--bare", repositoryRemoteURL]);
    git(["init", "--initial-branch=main", repositoryPath]);
    writeFileSync(path.join(repositoryPath, "README.md"), "seed\n");
    git(["-C", repositoryPath, "add", "README.md"]);
    git([
      "-C",
      repositoryPath,
      "-c",
      "user.name=E2E",
      "-c",
      "user.email=e2e@example.test",
      "-c",
      "commit.gpgsign=false",
      "commit",
      "-m",
      "seed",
    ]);
    git(["-C", repositoryPath, "remote", "add", "origin", repositoryRemoteURL]);
    git(["-C", repositoryPath, "push", "--set-upstream", "origin", "main"]);
    git(["-C", repositoryPath, "remote", "remove", "origin"]);
    run({ repositoryPath, repositoryRemoteURL });
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

describe("seed origin ownership", () => {
  it("restores the URL without fetching until repository work is quiescent", () => {
    withSeedRepository((seed) => {
      const lock = path.join(seed.repositoryPath, ".git", "refs", "remotes", "origin", "main.lock");
      mkdirSync(path.dirname(lock), { recursive: true });
      writeFileSync(lock, "owned concurrent fetch\n");
      expect(() => restoreSeedRepositoryOrigin(seed, { refreshTrackingRefs: false })).not.toThrow();
      expect(
        execFileSync("git", ["-C", seed.repositoryPath, "remote", "get-url", "origin"], {
          encoding: "utf8",
        }).trim(),
      ).toBe(seed.repositoryRemoteURL);
      rmSync(lock);
      restoreSeedRepositoryOrigin(seed);
      const tracked = execFileSync(
        "git",
        ["-C", seed.repositoryPath, "rev-parse", "refs/remotes/origin/main"],
        { encoding: "utf8" },
      );
      const local = execFileSync("git", ["-C", seed.repositoryPath, "rev-parse", "main"], {
        encoding: "utf8",
      });
      expect(tracked).toBe(local);
    });
  });

  it("preserves Git failure evidence when an explicit refresh fails", () => {
    withSeedRepository((seed) => {
      const lock = path.join(seed.repositoryPath, ".git", "refs", "remotes", "origin", "main.lock");
      mkdirSync(path.dirname(lock), { recursive: true });
      writeFileSync(lock, "owned concurrent fetch\n");
      expect(() => restoreSeedRepositoryOrigin(seed)).toThrow(/cannot lock ref/);
    });
  });
});
