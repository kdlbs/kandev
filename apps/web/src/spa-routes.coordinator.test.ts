import { describe, expect, it } from "vitest";
import { resolveSpaRoute } from "./spa-routes";

describe("resolveSpaRoute coordinator routes", () => {
  it("resolves the generic route to the needs-you view with no coordinator id", () => {
    expect(
      resolveSpaRoute("/workspaces/ws%2Fone/coordinator", new URLSearchParams(), {
        coordinatorEnabled: true,
      }),
    ).toEqual({
      kind: "coordinator",
      workspaceId: "ws/one",
      coordinatorId: null,
      view: "needs-you",
    });
  });

  it("resolves a coordinator id to the needs-you view", () => {
    expect(
      resolveSpaRoute("/workspaces/ws-1/coordinator/co%2F1", new URLSearchParams(), {
        coordinatorEnabled: true,
      }),
    ).toEqual({
      kind: "coordinator",
      workspaceId: "ws-1",
      coordinatorId: "co/1",
      view: "needs-you",
    });
  });

  it("resolves the queue suffix to the queue view", () => {
    expect(
      resolveSpaRoute("/workspaces/ws-1/coordinator/co-1/queue", new URLSearchParams(), {
        coordinatorEnabled: true,
      }),
    ).toEqual({
      kind: "coordinator",
      workspaceId: "ws-1",
      coordinatorId: "co-1",
      view: "queue",
    });
  });

  it("does not resolve coordinator routes while the feature is disabled", () => {
    expect(resolveSpaRoute("/workspaces/ws-1/coordinator", new URLSearchParams()).kind).not.toBe(
      "coordinator",
    );
    expect(
      resolveSpaRoute("/workspaces/ws-1/coordinator/co-1/queue", new URLSearchParams()).kind,
    ).not.toBe("coordinator");
  });

  it("does not treat a deeper path as a coordinator route", () => {
    expect(
      resolveSpaRoute("/workspaces/ws-1/coordinator/co-1/queue/extra", new URLSearchParams(), {
        coordinatorEnabled: true,
      }).kind,
    ).toBe("kanban");
  });
});
