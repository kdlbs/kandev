import { execFileSync, spawnSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { expect, it } from "vitest";
import { clearSeedRepositoryRemoteDefault } from "../fixtures/seed-repository-origin";

it("forgets the old remote default without deleting either main branch", () => {
  const repositoryPath = mkdtempSync(path.join(tmpdir(), "kandev-remote-default-"));
  const env = { ...process.env, GIT_CONFIG_NOSYSTEM: "1" };
  const git = (...args: string[]) =>
    execFileSync("git", ["-C", repositoryPath, ...args], { env, encoding: "utf8" }).trim();
  try {
    git("init", "--initial-branch=main");
    writeFileSync(path.join(repositoryPath, "README.md"), "seed\n");
    git("add", "README.md");
    git(
      "-c",
      "user.name=E2E",
      "-c",
      "user.email=e2e@example.test",
      "-c",
      "commit.gpgsign=false",
      "commit",
      "-m",
      "seed",
    );
    const oid = git("rev-parse", "HEAD");
    git("update-ref", "refs/remotes/origin/main", oid);
    git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main");
    clearSeedRepositoryRemoteDefault({ repositoryPath }, env);
    expect(
      spawnSync("git", ["-C", repositoryPath, "symbolic-ref", "-q", "refs/remotes/origin/HEAD"], {
        env,
      }).status,
    ).not.toBe(0);
    expect(git("rev-parse", "refs/remotes/origin/main")).toBe(oid);
    expect(git("rev-parse", "refs/heads/main")).toBe(oid);
    expect(() => clearSeedRepositoryRemoteDefault({ repositoryPath }, env)).not.toThrow();
  } finally {
    rmSync(repositoryPath, { recursive: true, force: true });
  }
});
