import { describe, expect, it, vi } from "vitest";
import { execFileSync } from "node:child_process";
import { DatabaseSync } from "node:sqlite";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import type { Page } from "@playwright/test";
import type { Repository } from "../../lib/types/http";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import {
  cleanupMultiRepoManagedCloneRelocationFixture,
  seedLegacyGenericSessionError,
  seedMultiRepoManagedCloneRelocationFixture,
  type MultiRepoRelocationFixture,
} from "./multi-repo-managed-clone-recovery";

describe("cleanupMultiRepoManagedCloneRelocationFixture", () => {
  it("attempts every restoration step and preserves the first failure", async () => {
    const firstFailure = new Error("workspace reset failed");
    const calls: string[] = [];
    const apiClient = {
      e2eReset: vi.fn(async () => {
        calls.push("reset");
        throw firstFailure;
      }),
      mockGitHubReset: vi.fn(async () => {
        calls.push("mock-reset");
        throw new Error("mock reset failed");
      }),
      deleteRepository: vi.fn(async () => {
        calls.push("delete-extra");
        throw new Error("delete extra repository failed");
      }),
      updateRepository: vi.fn(async () => {
        calls.push("restore-seed");
      }),
    } as unknown as ApiClient;
    const seedData = {
      workspaceId: "workspace",
      workflowId: "workflow",
      repositoryId: "seed-repository",
    } as SeedData;
    const originalSeedRepository = {
      id: "seed-repository",
      source_type: "local",
      local_path: "/seed/repository",
      provider: "",
      provider_repo_id: "",
      provider_owner: "",
      provider_name: "",
      remote_url: "https://example.test/repository.git",
      default_branch: "main",
      pull_before_worktree: true,
      setup_script: "",
      cleanup_script: "",
      dev_script: "",
      copy_files: "",
    } as Repository;
    const fixture = {
      originalSeedRepository,
      slots: [{}, { repositoryId: "extra-repository" }],
    } as unknown as MultiRepoRelocationFixture;

    await expect(
      cleanupMultiRepoManagedCloneRelocationFixture(apiClient, seedData, fixture),
    ).rejects.toBe(firstFailure);

    expect(calls).toEqual(["reset", "mock-reset", "delete-extra", "restore-seed"]);
    expect(apiClient.updateRepository).toHaveBeenCalledWith(
      "seed-repository",
      expect.objectContaining({
        source_type: "local",
        local_path: "/seed/repository",
        remote_url: "https://example.test/repository.git",
      }),
    );
  });
});

describe("seedLegacyGenericSessionError", () => {
  it("keeps a stopped resumable executor row while replacing the legacy session error", () => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "legacy-session-error-"));
    const db = new DatabaseSync(path.join(tmpDir, "kandev.db"));
    try {
      db.exec(`
        CREATE TABLE task_sessions (
          id TEXT PRIMARY KEY,
          metadata TEXT,
          state TEXT,
          error_message TEXT,
          updated_at TEXT
        );
        CREATE TABLE executors_running (
          session_id TEXT PRIMARY KEY,
          status TEXT,
          local_pid INTEGER
        );
        INSERT INTO task_sessions (id, metadata, state) VALUES ('session-1', '{}', 'CANCELLED');
        INSERT INTO executors_running (session_id, status, local_pid)
        VALUES ('session-1', 'stopped', 0);
      `);
    } finally {
      db.close();
    }

    try {
      seedLegacyGenericSessionError(tmpDir, "session-1");
      const check = new DatabaseSync(path.join(tmpDir, "kandev.db"));
      try {
        const session = check
          .prepare("SELECT state, error_message FROM task_sessions WHERE id = ?")
          .get("session-1") as { state: string; error_message: string };
        const executor = check
          .prepare("SELECT status, local_pid FROM executors_running WHERE session_id = ?")
          .get("session-1") as { status: string; local_pid: number };
        expect(session.state).toBe("FAILED");
        expect(session.error_message).toBe("The previous agent launch failed.");
        expect(executor).toEqual({ status: "stopped", local_pid: 0 });
      } finally {
        check.close();
      }
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  it("refuses to seed a session while its executor still claims a live process", () => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "legacy-session-error-"));
    const db = new DatabaseSync(path.join(tmpDir, "kandev.db"));
    try {
      db.exec(`
        CREATE TABLE task_sessions (
          id TEXT PRIMARY KEY,
          metadata TEXT,
          state TEXT,
          error_message TEXT,
          updated_at TEXT
        );
        CREATE TABLE executors_running (
          session_id TEXT PRIMARY KEY,
          status TEXT,
          local_pid INTEGER
        );
        INSERT INTO task_sessions (id, metadata, state) VALUES ('session-1', '{}', 'CANCELLED');
        INSERT INTO executors_running (session_id, status, local_pid)
        VALUES ('session-1', 'running', 4123);
      `);
    } finally {
      db.close();
    }

    try {
      expect(() => seedLegacyGenericSessionError(tmpDir, "session-1")).toThrow(
        "legacy recovery session session-1 still has a live execution",
      );
      const check = new DatabaseSync(path.join(tmpDir, "kandev.db"));
      try {
        const session = check
          .prepare("SELECT state, error_message FROM task_sessions WHERE id = ?")
          .get("session-1") as { state: string; error_message: string | null };
        expect(session).toEqual({ state: "CANCELLED", error_message: null });
      } finally {
        check.close();
      }
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });
});

describe("seedMultiRepoManagedCloneRelocationFixture", () => {
  it("restores the shared repository when setup fails after the first update", async () => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "multi-repo-recovery-"));
    const repositoryPath = path.join(tmpDir, "seed-repository");
    fs.mkdirSync(repositoryPath, { recursive: true });
    execFileSync("git", ["init", "-b", "main"], { cwd: repositoryPath, stdio: "pipe" });
    execFileSync("git", ["config", "user.email", "e2e@test.local"], {
      cwd: repositoryPath,
      stdio: "pipe",
    });
    execFileSync("git", ["config", "user.name", "E2E Test"], {
      cwd: repositoryPath,
      stdio: "pipe",
    });
    fs.writeFileSync(path.join(repositoryPath, "README.md"), "seed\n");
    execFileSync("git", ["add", "README.md"], { cwd: repositoryPath, stdio: "pipe" });
    execFileSync("git", ["commit", "-m", "initial"], {
      cwd: repositoryPath,
      stdio: "pipe",
    });

    const setupFailure = new Error("repository update failed after server mutation");
    const originalRepository = {
      id: "seed-repository",
      source_type: "local",
      local_path: repositoryPath,
      provider: "",
      provider_repo_id: "",
      provider_owner: "",
      provider_name: "",
      remote_url: "https://example.test/repository.git",
      default_branch: "main",
      pull_before_worktree: true,
      setup_script: "",
      cleanup_script: "",
      dev_script: "",
      copy_files: "",
    } as Repository;
    let updates = 0;
    const apiClient = {
      mockGitHubReset: vi.fn(async () => {}),
      getRepository: vi.fn(async () => originalRepository),
      updateRepository: vi.fn(async () => {
        updates += 1;
        if (updates === 1) throw setupFailure;
      }),
      e2eReset: vi.fn(async () => {}),
      deleteRepository: vi.fn(async () => {}),
    } as unknown as ApiClient;
    const seedData = {
      workspaceId: "workspace",
      workflowId: "workflow",
      repositoryId: originalRepository.id,
      repositoryPath,
      repositoryRemoteURL: originalRepository.remote_url,
      agentProfileId: "profile",
      startStepId: "step",
      worktreeExecutorProfileId: "executor",
    } as SeedData;

    try {
      await expect(
        seedMultiRepoManagedCloneRelocationFixture(
          {} as Page,
          apiClient,
          seedData,
          { tmpDir },
          "setup failure",
        ),
      ).rejects.toBe(setupFailure);

      expect(apiClient.e2eReset).toHaveBeenCalledOnce();
      expect(apiClient.mockGitHubReset).toHaveBeenCalledTimes(2);
      expect(apiClient.updateRepository).toHaveBeenCalledTimes(2);
      expect(apiClient.updateRepository).toHaveBeenLastCalledWith(
        originalRepository.id,
        expect.objectContaining({ local_path: repositoryPath, source_type: "local" }),
      );
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });
});
