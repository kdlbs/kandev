import { describe, expect, it } from "vitest";

import { getInitialPageProps } from "./spa-routing";

describe("getInitialPageProps", () => {
  it("hydrates task detail URLs into the existing kanban page client", () => {
    expect(
      getInitialPageProps({
        route: {
          kind: "spa",
          route: "taskDetail",
          path: "/t/task-1",
          params: { taskId: "task-1" },
        },
        initialState: {},
      }),
    ).toEqual({ initialTaskId: "task-1" });
  });

  it("renders the root kanban page without task focus for non-task routes", () => {
    expect(
      getInitialPageProps({
        route: { kind: "spa", route: "home", path: "/" },
        initialState: {},
      }),
    ).toEqual({});
  });
});
