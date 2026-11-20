import { describe, expect, it } from "vitest";
import {
  captureUserShellPublicationOwner,
  isActiveUserShellPublicationOwner,
  isActiveUserShellScope,
  makeUserShellScope,
  mergeUserShellsState,
  resolveActiveUserShellScope,
  userShellScopeKey,
} from "./user-shell-scope";
import type { UserShellsState } from "./slices/session-runtime/types";

const ENVIRONMENT = "env-1";
const SESSION = "session-1";

function state(activeTaskId: string | null, environmentId = ENVIRONMENT) {
  return {
    tasks: { activeSessionId: SESSION, activeTaskId },
    environmentIdBySessionId: { [SESSION]: environmentId },
  };
}

describe("user shell scope", () => {
  it("encodes a taskless read distinctly from a task read", () => {
    expect(userShellScopeKey({ environmentId: ENVIRONMENT, taskId: null })).not.toBe(
      userShellScopeKey({ environmentId: ENVIRONMENT, taskId: "task-1" }),
    );
  });

  it("resolves the active task and environment", () => {
    expect(resolveActiveUserShellScope(state("task-1"))).toEqual({
      environmentId: ENVIRONMENT,
      taskId: "task-1",
    });
    expect(resolveActiveUserShellScope(state(null))?.taskId).toBeNull();
    expect(resolveActiveUserShellScope({ environmentIdBySessionId: {} })).toBeNull();
  });

  it("accepts only the visible task/environment scope", () => {
    const taskA = makeUserShellScope(ENVIRONMENT, "task-a")!;
    const taskB = makeUserShellScope(ENVIRONMENT, "task-b")!;
    expect(isActiveUserShellScope(state("task-a"), taskA)).toBe(true);
    expect(isActiveUserShellScope(state("task-b"), taskA)).toBe(false);
    expect(isActiveUserShellScope(state("task-a"), taskB)).toBe(false);
    expect(isActiveUserShellScope(state(null), taskA)).toBe(false);
    expect(isActiveUserShellScope(state("task-a"), null)).toBe(false);
  });

  it("rejects a task/environment pair from another environment", () => {
    const otherEnvironment = makeUserShellScope("env-2", "task-a")!;
    expect(isActiveUserShellScope(state("task-a"), otherEnvironment)).toBe(false);
  });

  it("rejects a late publication after a workspace context replacement", () => {
    const scope = makeUserShellScope(ENVIRONMENT, "task-a")!;
    const base = { ...state("task-a"), workspaces: { activeId: "workspace-1" } };
    const owner = captureUserShellPublicationOwner(base, scope);
    expect(isActiveUserShellPublicationOwner(base, owner)).toBe(true);
    expect(
      isActiveUserShellPublicationOwner({ ...base, workspaceContextGeneration: 1 }, owner),
    ).toBe(false);
    expect(
      isActiveUserShellPublicationOwner(
        { ...base, workspaces: { activeId: "workspace-2" } },
        owner,
      ),
    ).toBe(false);
    expect(isActiveUserShellPublicationOwner(state("task-b"), owner)).toBe(false);
  });

  it("keeps every shell namespace while merging hydration state", () => {
    const defaults: UserShellsState = {
      byScope: { a: [] },
      byEnvironmentId: { e: [] },
      dismissedByScope: { a: { t: true } },
      dismissedByEnvironmentId: { e: { s: true } },
      loading: { a: false },
      loaded: { a: true },
      listed: { a: true },
      sharedGeneration: { e: 2 },
    };
    const merged = mergeUserShellsState(defaults, {
      byScope: { b: [] },
      loading: { b: true },
    });
    expect(merged.byScope).toEqual({ a: [], b: [] });
    expect(merged.byEnvironmentId).toEqual({ e: [] });
    expect(merged.dismissedByScope).toEqual({ a: { t: true } });
    expect(merged.dismissedByEnvironmentId).toEqual({ e: { s: true } });
    expect(merged.loading).toEqual({ a: false, b: true });
    expect(merged.loaded).toEqual({ a: true });
    expect(merged.listed).toEqual({ a: true });
    expect(merged.sharedGeneration).toEqual({ e: 2 });
  });
});
