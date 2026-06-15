import { describe, expect, it } from "vitest";

import { getInitialPageProps } from "./spa-routing";
import { resolveSpaRoute } from "./spa-routes";

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

describe("resolveSpaRoute", () => {
  it("maps task detail paths to the kanban page focus props", () => {
    expect(resolveSpaRoute("/t/task-1", new URLSearchParams())).toEqual({
      kind: "kanban",
      taskId: "task-1",
      sessionId: undefined,
    });
    expect(resolveSpaRoute("/tasks/task-2", new URLSearchParams("sessionId=sess-1"))).toEqual({
      kind: "kanban",
      taskId: "task-2",
      sessionId: "sess-1",
    });
  });

  it("maps first-class SPA surfaces to their route keys", () => {
    expect(resolveSpaRoute("/tasks", new URLSearchParams())).toEqual({ kind: "tasks" });
    expect(resolveSpaRoute("/github", new URLSearchParams())).toEqual({ kind: "github" });
    expect(resolveSpaRoute("/gitlab", new URLSearchParams())).toEqual({ kind: "gitlab" });
    expect(resolveSpaRoute("/jira", new URLSearchParams())).toEqual({ kind: "jira" });
    expect(resolveSpaRoute("/linear", new URLSearchParams())).toEqual({ kind: "linear" });
  });

  it("normalizes stats range query params", () => {
    expect(resolveSpaRoute("/stats", new URLSearchParams("range=week"))).toEqual({
      kind: "stats",
      range: "week",
    });
    expect(resolveSpaRoute("/stats", new URLSearchParams("range=bad"))).toEqual({
      kind: "stats",
      range: undefined,
    });
  });
});
