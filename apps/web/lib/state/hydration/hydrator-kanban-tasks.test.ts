import { describe, expect, it } from "vitest";
import { produce } from "immer";
import type { Draft } from "immer";
import { hydrateState } from "./hydrator";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";

function makeAppDraft(): AppState {
  return structuredClone(defaultState) as AppState;
}

describe("hydrateState — kanban task executor field preservation", () => {
  const taskId = "task-1";
  const knownExecutor = {
    primaryExecutorId: "exec-1",
    primaryExecutorType: "worktree",
    primaryExecutorName: "Worktree",
    isRemoteExecutor: false,
  };

  it("keeps known executor fields when a same-timestamp merge omits them", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      hydrateState(draft, {
        kanban: {
          tasks: [
            {
              id: taskId,
              updatedAt: "2026-08-15T00:00:00.000Z",
              ...knownExecutor,
            },
          ],
        },
      } as unknown as Partial<AppState>);

      hydrateState(draft, {
        kanban: {
          tasks: [
            {
              id: taskId,
              updatedAt: "2026-08-15T00:00:00.000Z",
              title: "Refreshed title",
            },
          ],
        },
      } as unknown as Partial<AppState>);
    });

    const merged = result.kanban.tasks.find((t) => t.id === taskId);
    expect(merged).toMatchObject(knownExecutor);
    expect(merged?.title).toBe("Refreshed title");
  });

  it("adopts a legitimately different executor value instead of preserving the old one", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      hydrateState(draft, {
        kanban: {
          tasks: [
            {
              id: taskId,
              updatedAt: "2026-08-15T00:00:00.000Z",
              ...knownExecutor,
            },
          ],
        },
      } as unknown as Partial<AppState>);

      hydrateState(draft, {
        kanban: {
          tasks: [
            {
              id: taskId,
              updatedAt: "2026-08-15T00:01:00.000Z",
              primaryExecutorId: "exec-2",
              primaryExecutorType: "ssh",
              primaryExecutorName: "Remote box",
              isRemoteExecutor: true,
            },
          ],
        },
      } as unknown as Partial<AppState>);
    });

    const merged = result.kanban.tasks.find((t) => t.id === taskId);
    expect(merged).toMatchObject({
      primaryExecutorId: "exec-2",
      primaryExecutorType: "ssh",
      primaryExecutorName: "Remote box",
      isRemoteExecutor: true,
    });
  });
});
