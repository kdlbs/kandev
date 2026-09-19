import { describe, expect, it } from "vitest";
import { produce } from "immer";
import type { Draft } from "immer";
import { hydrateState } from "./hydrator";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";

function makeAppDraft(): AppState {
  return structuredClone(defaultState) as AppState;
}

const taskId = "task-1";
const updatedAt = "2026-08-15T00:00:00.000Z";
const knownExecutor = {
  primaryExecutorId: "exec-1",
  primaryExecutorType: "worktree",
  primaryExecutorName: "Worktree",
  isRemoteExecutor: false,
};

function seedKnownExecutorTask(draft: Draft<AppState>): void {
  hydrateState(draft, {
    kanban: {
      tasks: [
        {
          id: taskId,
          updatedAt,
          ...knownExecutor,
        },
      ],
    },
  } as unknown as Partial<AppState>);
}

function hydrateFrozenIncoming(
  incoming: Readonly<{ id: string; updatedAt: string; title: string }>,
): AppState {
  return produce(makeAppDraft(), (draft: Draft<AppState>) => {
    seedKnownExecutorTask(draft);

    hydrateState(draft, {
      kanban: {
        tasks: [incoming],
      },
    } as unknown as Partial<AppState>);
  });
}

describe("hydrateState — kanban task executor field preservation", () => {
  it("keeps known executor fields when a same-timestamp merge omits them", () => {
    const result = produce(makeAppDraft(), (draft: Draft<AppState>) => {
      seedKnownExecutorTask(draft);

      hydrateState(draft, {
        kanban: {
          tasks: [
            {
              id: taskId,
              updatedAt,
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
      seedKnownExecutorTask(draft);

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

  it("clones a frozen incoming task before preserving omitted executor fields", () => {
    const incoming = Object.freeze({
      id: taskId,
      updatedAt,
      title: "Frozen snapshot task",
    });

    expect(() => hydrateFrozenIncoming(incoming)).not.toThrow();
    const result = hydrateFrozenIncoming(incoming);
    const merged = result.kanban.tasks.find((t) => t.id === taskId);
    expect(merged).toMatchObject({
      title: "Frozen snapshot task",
      ...knownExecutor,
    });
  });

  it("clones a newer frozen task before preserving omitted executor fields", () => {
    const incoming = Object.freeze({
      id: taskId,
      updatedAt: "2026-08-15T00:01:00.000Z",
      title: "Newer frozen snapshot task",
    });

    expect(() => hydrateFrozenIncoming(incoming)).not.toThrow();
    const result = hydrateFrozenIncoming(incoming);
    const merged = result.kanban.tasks.find((t) => t.id === taskId);
    expect(merged).toMatchObject({
      title: "Newer frozen snapshot task",
      updatedAt: "2026-08-15T00:01:00.000Z",
      ...knownExecutor,
    });
  });
});
