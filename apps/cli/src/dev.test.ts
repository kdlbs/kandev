import path from "node:path";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resolveDevBackendEnv } from "./dev";

describe("resolveDevBackendEnv", () => {
  const originalTaskId = process.env.KANDEV_TASK_ID;
  const originalDbPath = process.env.KANDEV_DATABASE_PATH;
  const originalMock = process.env.KANDEV_MOCK_AGENT;

  beforeEach(() => {
    delete process.env.KANDEV_TASK_ID;
    delete process.env.KANDEV_DATABASE_PATH;
    delete process.env.KANDEV_MOCK_AGENT;
    vi.spyOn(console, "log").mockImplementation(() => {});
  });

  afterEach(() => {
    restoreEnv("KANDEV_TASK_ID", originalTaskId);
    restoreEnv("KANDEV_DATABASE_PATH", originalDbPath);
    restoreEnv("KANDEV_MOCK_AGENT", originalMock);
    vi.restoreAllMocks();
  });

  it("defaults to <repo>/.kandev-dev/data/kandev.db in a plain shell", () => {
    const repoRoot = "/tmp/repo";
    const { dbPath, extra } = resolveDevBackendEnv(repoRoot);

    expect(dbPath).toBe(path.join(repoRoot, ".kandev-dev", "data", "kandev.db"));
    expect(extra.KANDEV_HOME_DIR).toBe(path.join(repoRoot, ".kandev-dev"));
    expect(extra.KANDEV_DATABASE_PATH).toBe("");
  });

  it("honors an explicit KANDEV_DATABASE_PATH override in a plain shell", () => {
    process.env.KANDEV_DATABASE_PATH = "/tmp/custom.db";

    const { dbPath, extra } = resolveDevBackendEnv("/tmp/repo");

    expect(dbPath).toBe("/tmp/custom.db");
    expect(extra.KANDEV_DATABASE_PATH).toBe("/tmp/custom.db");
    expect(extra.KANDEV_HOME_DIR).toBeUndefined();
  });

  it("ignores a leaked KANDEV_DATABASE_PATH when running inside a kandev task", () => {
    process.env.KANDEV_TASK_ID = "fake-task-id";
    process.env.KANDEV_DATABASE_PATH = "/parent/kandev.db";
    const repoRoot = "/tmp/repo";

    const { dbPath, extra } = resolveDevBackendEnv(repoRoot);

    expect(dbPath).toBe(path.join(repoRoot, ".kandev-dev", "data", "kandev.db"));
    expect(extra.KANDEV_HOME_DIR).toBe(path.join(repoRoot, ".kandev-dev"));
    expect(extra.KANDEV_DATABASE_PATH).toBe("");
  });
});

function restoreEnv(name: string, value: string | undefined): void {
  if (value === undefined) {
    delete process.env[name];
  } else {
    process.env[name] = value;
  }
}
