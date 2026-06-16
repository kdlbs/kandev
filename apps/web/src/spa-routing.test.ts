import { describe, expect, it } from "vitest";

import { officeRouteKey } from "./office-routes";
import { getInitialPageProps } from "./spa-routing";
import { resolveSpaRoute } from "./spa-routes";
import { settingsRouteKey } from "./settings-routes";

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

  it("routes settings and office paths through their SPA shells", () => {
    expect(resolveSpaRoute("/settings/general/editors", new URLSearchParams())).toEqual({
      kind: "settings",
      pathname: "/settings/general/editors",
    });
    expect(resolveSpaRoute("/office/projects/project-1", new URLSearchParams())).toEqual({
      kind: "office",
      pathname: "/office/projects/project-1",
    });
    expect(resolveSpaRoute("/office/setup", new URLSearchParams("mode=new"))).toEqual({
      kind: "office",
      pathname: "/office/setup",
    });
  });
});

describe("settingsRouteKey", () => {
  it("normalizes dynamic settings detail paths", () => {
    expect(settingsRouteKey("/settings/workspace/ws-1/repositories/")).toBe(
      "/settings/workspace/ws-1/repositories",
    );
    expect(settingsRouteKey("/settings/executor/executor-1/profile/profile-1")).toBe(
      "/settings/executor/executor-1/profile/profile-1",
    );
    expect(settingsRouteKey("/settings/executors/new/ssh")).toBe("/settings/executors/new/ssh");
  });
});

describe("officeRouteKey", () => {
  it("normalizes dynamic office detail paths", () => {
    expect(officeRouteKey("/office/agents/agent-1/configuration/")).toBe(
      "/office/agents/agent-1/configuration",
    );
    expect(officeRouteKey("/office/agents/agent-1/runs/run-1")).toBe(
      "/office/agents/agent-1/runs/run-1",
    );
    expect(officeRouteKey("/office/routines/routine-1")).toBe("/office/routines/routine-1");
    expect(officeRouteKey("/office/setup/")).toBe("/office/setup");
  });
});
