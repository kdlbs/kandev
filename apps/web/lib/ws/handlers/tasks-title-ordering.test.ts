import { describe, expect, it } from "vitest";
import { registerTasksHandlers } from "./tasks";
import { makeMessage, makeStore, makeTask } from "./tasks.test-helpers";

describe("task title event ordering", () => {
  it("keeps a renamed sender when an older same-millisecond event arrives later", () => {
    const store = makeStore();
    const update = registerTasksHandlers(store)["task.updated"]!;
    const sender = makeTask("sender", null);

    update(
      makeMessage({
        ...sender,
        title: "Original sender title",
        updated_at: "2026-10-09T10:00:00.123456700Z",
      }),
    );
    update(
      makeMessage({
        ...sender,
        title: "Renamed sender",
        updated_at: "2026-10-09T10:00:00.123456900Z",
      }),
    );
    update(
      makeMessage({
        ...sender,
        title: "Original sender title",
        updated_at: "2026-10-09T10:00:00.123456800Z",
      }),
    );

    expect(store.getState().kanban.tasks[0].title).toBe("Renamed sender");
    expect(store.getState().kanbanMulti.snapshots.wf1.tasks[0].title).toBe("Renamed sender");

    update(
      makeMessage({
        ...sender,
        title: "Newest sender",
        updated_at: "2026-10-09T10:00:00.123456999Z",
      }),
    );
    expect(store.getState().kanban.tasks[0].title).toBe("Newest sender");
  });

  it("accepts an update without a comparable timestamp", () => {
    const store = makeStore();
    const update = registerTasksHandlers(store)["task.updated"]!;
    const sender = makeTask("sender", null);
    update(makeMessage({ ...sender, title: "Original", updated_at: "2026-10-09T10:00:00Z" }));
    update(makeMessage({ ...sender, title: "Renamed" }));
    expect(store.getState().kanban.tasks[0].title).toBe("Renamed");
  });
});
