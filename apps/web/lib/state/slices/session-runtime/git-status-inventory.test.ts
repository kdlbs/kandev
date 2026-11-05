import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createSessionRuntimeSlice } from "./session-runtime-slice";
import type { GitStatusEntry, SessionRuntimeSlice } from "./types";
import {
  gitStatusInventoryGeneration,
  isCurrentGitStatusScope,
  isUnknownGitStatusScope,
} from "./git-status-inventory";

const ENVIRONMENT = "env-attachment";
const MASTER = "master";
const SECONDARY = "secondary";

function entry(repositoryName: string, modified: string[] = []): GitStatusEntry {
  const files: GitStatusEntry["files"] = {};
  for (const path of modified) files[path] = { path, status: "modified", staged: false };
  return {
    branch: "main",
    remote_branch: null,
    modified,
    added: [],
    deleted: [],
    untracked: [],
    renamed: [],
    ahead: 0,
    behind: 0,
    files,
    timestamp: new Date().toISOString() + Math.random(),
    repository_name: repositoryName,
  };
}

function makeStore() {
  return create<SessionRuntimeSlice>()(immer((set, get) => createSessionRuntimeSlice(set, get)));
}

function seedMultiRepositoryState(store: ReturnType<typeof makeStore>) {
  store.getState().setGitStatus(ENVIRONMENT, entry("", ["root.md"]));
  store.getState().setGitStatus(ENVIRONMENT, entry(MASTER, ["master.md"]));
  store.getState().setGitStatus(ENVIRONMENT, entry(SECONDARY, ["secondary.md"]));
  store.getState().setGitStatusRefresh(ENVIRONMENT, "", {
    state: "unavailable",
    error_code: "repository_unavailable",
  });
  store.getState().setGitStatusRefresh(ENVIRONMENT, SECONDARY, {
    state: "unavailable",
    error_code: "details_unavailable",
  });
  store.getState().setGitStatusRefresh(ENVIRONMENT, undefined, {
    state: "unavailable",
    error_code: "status_unavailable",
    request_id: "git-refresh-settled",
  });
}

describe("git status inventory invalidation", () => {
  it("advances once and preserves healthy snapshots while scopes are unknown", () => {
    const store = makeStore();
    seedMultiRepositoryState(store);

    store.getState().invalidateGitStatusInventory(ENVIRONMENT);
    store.getState().invalidateGitStatusInventory(ENVIRONMENT);

    const state = store.getState();
    expect(gitStatusInventoryGeneration(state, ENVIRONMENT)).toBe(2);
    expect(state.gitStatus.inventoryByEnvironment?.[ENVIRONMENT]?.scopes).toBeNull();
    expect(Object.keys(state.gitStatus.byEnvironmentRepo[ENVIRONMENT]).sort()).toEqual([
      "",
      MASTER,
      SECONDARY,
    ]);
  });
});

describe("git status inventory reconciliation", () => {
  it("retires the unnamed scope without touching current named snapshots", () => {
    const store = makeStore();
    seedMultiRepositoryState(store);
    store.getState().invalidateGitStatusInventory(ENVIRONMENT);
    const generation = gitStatusInventoryGeneration(store.getState(), ENVIRONMENT);

    // Establish the unnamed root first so the next aggregate can retire it.
    expect(
      store
        .getState()
        .reconcileGitStatusInventory(ENVIRONMENT, generation, ["", MASTER, SECONDARY]),
    ).toBe(true);
    const applied = store
      .getState()
      .reconcileGitStatusInventory(ENVIRONMENT, generation, [SECONDARY, MASTER]);

    expect(applied).toBe(true);
    const state = store.getState();
    expect(state.gitStatus.inventoryByEnvironment?.[ENVIRONMENT]).toMatchObject({
      generation,
      scopes: [MASTER, SECONDARY],
      retiredScopes: [""],
    });
    expect(Object.keys(state.gitStatus.byEnvironmentRepo[ENVIRONMENT]).sort()).toEqual([
      MASTER,
      SECONDARY,
    ]);
    expect(state.gitStatus.byEnvironmentId[ENVIRONMENT]?.repository_name).not.toBe("");
    expect(state.gitStatus.refreshByEnvironmentRepo?.[ENVIRONMENT]?.[""]).toBeUndefined();
    expect(state.gitStatus.refreshByEnvironmentRepo?.[ENVIRONMENT]?.[SECONDARY]).toEqual({
      state: "unavailable",
      error_code: "details_unavailable",
    });
    expect(state.gitStatusDisplay.byEnvironmentRepo[ENVIRONMENT]?.[""]).toBeUndefined();
    expect(state.gitStatus.refreshByEnvironmentId?.[ENVIRONMENT]).toBeUndefined();
  });

  it("keeps a valid unnamed root beside named children", () => {
    const store = makeStore();
    seedMultiRepositoryState(store);

    const applied = store
      .getState()
      .reconcileGitStatusInventory(ENVIRONMENT, 0, ["", MASTER, SECONDARY]);

    expect(applied).toBe(true);
    const state = store.getState();
    expect(state.gitStatus.byEnvironmentRepo[ENVIRONMENT]?.[""]).toBeDefined();
    expect(state.gitStatus.byEnvironmentId[ENVIRONMENT]?.repository_name).toBe(SECONDARY);
    expect(state.gitStatus.inventoryByEnvironment?.[ENVIRONMENT]?.retiredScopes).toEqual([]);
    expect(gitStatusInventoryGeneration(state, ENVIRONMENT)).toBe(0);
  });

  it("does not retire anything for a generation that already advanced", () => {
    const store = makeStore();
    seedMultiRepositoryState(store);
    store.getState().invalidateGitStatusInventory(ENVIRONMENT);

    const applied = store.getState().reconcileGitStatusInventory(ENVIRONMENT, 0, []);

    expect(applied).toBe(false);
    expect(Object.keys(store.getState().gitStatus.byEnvironmentRepo[ENVIRONMENT]).sort()).toEqual([
      "",
      MASTER,
      SECONDARY,
    ]);
  });

  it("rejects a retired scope while accepting a newly discovered one", () => {
    const store = makeStore();
    store.getState().reconcileGitStatusInventory(ENVIRONMENT, 0, ["", MASTER, SECONDARY]);
    store.getState().reconcileGitStatusInventory(ENVIRONMENT, 0, [MASTER, SECONDARY]);

    expect(isCurrentGitStatusScope(store.getState(), ENVIRONMENT, "")).toBe(false);
    expect(isCurrentGitStatusScope(store.getState(), ENVIRONMENT, MASTER)).toBe(true);
    expect(isCurrentGitStatusScope(store.getState(), ENVIRONMENT, "submodule/new")).toBe(true);
    expect(isUnknownGitStatusScope(store.getState(), ENVIRONMENT, "submodule/new")).toBe(true);
  });
});

describe("legacy git status retirement", () => {
  it("removes the unnamed refresh failure with the retired snapshot", () => {
    const store = makeStore();
    store.getState().registerSessionEnvironment("session-attachment", ENVIRONMENT);
    seedMultiRepositoryState(store);

    store.getState().clearLegacyGitStatusEntry("session-attachment");

    const state = store.getState();
    expect(state.gitStatus.byEnvironmentRepo[ENVIRONMENT]?.[""]).toBeUndefined();
    expect(state.gitStatus.refreshByEnvironmentRepo?.[ENVIRONMENT]?.[""]).toBeUndefined();
    expect(state.gitStatus.refreshByEnvironmentRepo?.[ENVIRONMENT]?.[SECONDARY]).toBeDefined();
    expect(state.gitStatus.refreshByEnvironmentId?.[ENVIRONMENT]).toBeUndefined();
    expect(state.gitStatus.byEnvironmentRepo[ENVIRONMENT]?.[MASTER]).toBeDefined();
  });
});
