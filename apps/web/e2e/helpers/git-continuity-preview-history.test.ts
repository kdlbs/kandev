import { createHash } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, expect, it, vi } from "vitest";

vi.mock("../pages/kanban-page", () => ({ KanbanPage: vi.fn() }));
vi.mock("../pages/session-page", () => ({ SessionPage: vi.fn() }));
import { GitHelper, makeGitEnv } from "./git-helper";
import { seedContinuityPreviewHistory } from "../tests/git/git-continuity-preview-history";

const OWNED_PATHS = [1, 2, 3].flatMap((number) => {
  const suffix = `0${number}-continuity-history`;
  return [
    `preview-${suffix}.html`,
    `preview-assets-${suffix}/preview.css`,
    `preview-assets-${suffix}/preview.js`,
    `preview-assets-${suffix}/logo.svg`,
  ];
});
const UNTRACKED_PATH = "preview-assets-02-continuity-history/unrelated.txt";

function withRepository(check: (git: GitHelper, directory: string, initialHead: string) => void) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-continuity-seed-"));
  const directory = path.join(root, "repo");
  const env = makeGitEnv(root);
  for (const key of Object.keys(env)) {
    if (key.startsWith("GIT_") && !/^GIT_(AUTHOR|COMMITTER)_(NAME|EMAIL)$/.test(key))
      delete env[key];
  }
  const config = path.join(root, "isolated.gitconfig");
  fs.writeFileSync(config, "");
  Object.assign(env, {
    GIT_CONFIG_NOSYSTEM: "1",
    GIT_CONFIG_GLOBAL: config,
    GIT_CONFIG_SYSTEM: config,
  });
  console.info(
    JSON.stringify({
      root,
      envSha256: createHash("sha256").update(JSON.stringify(env)).digest("hex"),
    }),
  );
  fs.mkdirSync(directory);
  const git = new GitHelper(directory, env);
  try {
    git.exec("git init --initial-branch=main");
    git.createFile("tracked-sentinel.txt", "tracked sentinel\n");
    git.exec('git add -- "tracked-sentinel.txt"');
    git.commit("seed private repository");
    const initialHead = git.getCurrentSha();
    git.createFile(UNTRACKED_PATH, "untracked sentinel\n");
    check(git, directory, initialHead);
  } finally {
    vi.restoreAllMocks();
    fs.rmSync(root, { recursive: true, force: true });
  }
}

function expectRestored(git: GitHelper, directory: string, initialHead: string) {
  expect.soft(git.getCurrentSha()).toBe(initialHead);
  expect
    .soft(git.exec(`git status --porcelain -- ${OWNED_PATHS.map((name) => `"${name}"`).join(" ")}`))
    .toBe("");
  expect
    .soft(fs.readFileSync(path.join(directory, "tracked-sentinel.txt"), "utf8"))
    .toBe("tracked sentinel\n");
  expect.soft(fs.existsSync(path.join(directory, UNTRACKED_PATH))).toBe(true);
  if (fs.existsSync(path.join(directory, UNTRACKED_PATH))) {
    expect
      .soft(fs.readFileSync(path.join(directory, UNTRACKED_PATH), "utf8"))
      .toBe("untracked sentinel\n");
  }
  expect.soft(git.exec(`git ls-files -- "${UNTRACKED_PATH}"`)).toBe("");
}

describe("seedContinuityPreviewHistory real Git rollback", () => {
  it.each([
    { phase: "early", failurePath: "preview-01-continuity-history.html" },
    {
      phase: "after an earlier commit",
      failurePath: "preview-assets-02-continuity-history/preview.css",
    },
  ])("restores only owned paths after $phase setup failure", ({ phase, failurePath }) => {
    withRepository((git, directory, initialHead) => {
      const setupError = new Error("preview write failed");
      const createFile = git.createFile.bind(git);
      let headAtFailure = "";
      vi.spyOn(git, "createFile").mockImplementation((name, content) => {
        createFile(name, content);
        if (name === failurePath) {
          headAtFailure = git.getCurrentSha();
          throw setupError;
        }
      });
      let caught: unknown;
      try {
        seedContinuityPreviewHistory(git);
      } catch (error) {
        caught = error;
      }
      expect(caught).toBe(setupError);
      if (phase === "early") expect(headAtFailure).toBe(initialHead);
      else expect(headAtFailure).not.toBe(initialHead);
      expectRestored(git, directory, initialHead);
    });
  });

  it("keeps the successful three-commit history and restores without capturing unrelated files", () => {
    withRepository((git, directory, initialHead) => {
      const restore = seedContinuityPreviewHistory(git);
      expect(git.exec(`git rev-list --count ${initialHead}..HEAD`).trim()).toBe("3");
      expect(fs.readFileSync(path.join(directory, OWNED_PATHS[0]), "utf8")).toBe(
        "<!doctype html><html><body><p>Saved source</p></body></html>",
      );
      expect.soft(git.exec(`git ls-files -- "${UNTRACKED_PATH}"`)).toBe("");
      restore();
      expectRestored(git, directory, initialHead);
    });
  });

  it.each(["reset", "delete"])(
    "retains setup and %s cleanup errors instead of claiming restoration",
    (failure) => {
      withRepository((git, directory, initialHead) => {
        const setupError = new Error("preview write failed");
        const cleanupError = new Error("preview cleanup failed");
        const createFile = git.createFile.bind(git);
        vi.spyOn(git, "createFile").mockImplementation((name, content) => {
          createFile(name, content);
          if (name === OWNED_PATHS[5]) throw setupError;
        });
        const exec = git.exec.bind(git);
        vi.spyOn(git, "exec").mockImplementation((command) => {
          if (failure === "reset" && command.startsWith("git reset --hard ")) throw cleanupError;
          return exec(command);
        });
        const deleteFile = git.deleteFile.bind(git);
        vi.spyOn(git, "deleteFile").mockImplementation((name) => {
          if (failure === "delete" && name === OWNED_PATHS[0]) throw cleanupError;
          deleteFile(name);
        });
        let caught: unknown;
        try {
          seedContinuityPreviewHistory(git);
        } catch (error) {
          caught = error;
        }
        expect(caught).toBeInstanceOf(AggregateError);
        expect((caught as AggregateError).errors).toEqual([setupError, cleanupError]);
        expect((caught as AggregateError).errors[0]).toBe(setupError);
        expect((caught as AggregateError).errors[1]).toBe(cleanupError);
        expect((caught as AggregateError).cause).toBe(setupError);
        expect(fs.existsSync(path.join(directory, OWNED_PATHS[5]))).toBe(false);
        if (failure === "reset") expect(git.getCurrentSha()).not.toBe(initialHead);
      });
    },
  );
});
