import { beforeEach, describe, expect, it } from "vitest";
import { setRecentTasks } from "@/lib/recent-tasks";
import { selectNextTaskAfterRemoval } from "./task-removal-candidates";
import type { TaskRow } from "./use-task-removal.test-utils";

const RECENT_TASK_VISITED_AT = "2026-06-07T10:00:00Z";
const CASCADE_PARENT: TaskRow = { id: "task-parent", primarySessionId: "sess-parent" };
const CASCADE_CHILD: TaskRow = {
  id: "task-child",
  primarySessionId: "sess-child",
  parentTaskId: CASCADE_PARENT.id,
};
const CASCADE_DEEP_CHILD: TaskRow = {
  id: "task-deep-child",
  primarySessionId: "sess-deep-child",
  parentTaskId: CASCADE_CHILD.id,
};
const CASCADE_CHILD_VISITED_AT = "2026-06-07T12:00:00Z";
const CASCADE_DEEP_CHILD_VISITED_AT = "2026-06-07T11:00:00Z";

const select = selectNextTaskAfterRemoval as unknown as (
  remaining: TaskRow[],
  removedTaskId: string,
  isLive: (taskId: string) => Promise<boolean>,
  options?: {
    excludedTaskIds?: ReadonlySet<string>;
    isTaskPendingRemoval?: (taskId: string) => boolean;
  },
) => Promise<TaskRow | null>;

beforeEach(() => {
  window.localStorage.clear();
});

describe("selectNextTaskAfterRemoval — stale-candidate rejection", () => {
  it("skips every excluded descendant while preserving recent-task order", async () => {
    const child: TaskRow = { ...CASCADE_CHILD, parentTaskId: null };
    const deepChild: TaskRow = { ...CASCADE_DEEP_CHILD, parentTaskId: null };
    const unrelated: TaskRow = { id: "task-unrelated", primarySessionId: "sess-unrelated" };
    setRecentTasks([
      { taskId: child.id, title: "Child", visitedAt: CASCADE_CHILD_VISITED_AT },
      { taskId: deepChild.id, title: "Deep child", visitedAt: CASCADE_DEEP_CHILD_VISITED_AT },
      { taskId: unrelated.id, title: "Unrelated", visitedAt: RECENT_TASK_VISITED_AT },
    ]);

    await expect(
      select([child, deepChild, unrelated], CASCADE_PARENT.id, async () => true, {
        excludedTaskIds: new Set([CASCADE_PARENT.id, child.id, deepChild.id]),
      }),
    ).resolves.toBe(unrelated);
  });

  it("returns null (redirect home) when the only remaining candidate is not a live board member", async () => {
    const stale: TaskRow = { id: "task-A-stale", primarySessionId: "sess-A" };

    await expect(select([stale], "task-B", async () => false)).resolves.toBeNull();
  });

  it("returns the candidate when it is a live board member", async () => {
    const live: TaskRow = { id: "task-A-live", primarySessionId: "sess-A" };

    await expect(select([live], "task-B", async () => true)).resolves.toBe(live);
  });
});

describe("selectNextTaskAfterRemoval — pending removal intent", () => {
  it("skips a candidate that enters another removal during its liveness read", async () => {
    const first: TaskRow = { id: "task-first", primarySessionId: "sess-first" };
    const second: TaskRow = { id: "task-second", primarySessionId: "sess-second" };
    const pending = new Set<string>();

    await expect(
      select(
        [first, second],
        "task-removed",
        async (taskId) => {
          if (taskId === first.id) pending.add(first.id);
          return true;
        },
        { isTaskPendingRemoval: (taskId) => pending.has(taskId) },
      ),
    ).resolves.toBe(second);
  });

  it("does not return a candidate that is already pending", async () => {
    const pendingTask: TaskRow = { id: "task-pending", primarySessionId: "sess-pending" };
    const eligible: TaskRow = { id: "task-eligible", primarySessionId: "sess-eligible" };

    await expect(
      select([pendingTask, eligible], "task-removed", async () => true, {
        isTaskPendingRemoval: (taskId) => taskId === pendingTask.id,
      }),
    ).resolves.toBe(eligible);
  });
});
