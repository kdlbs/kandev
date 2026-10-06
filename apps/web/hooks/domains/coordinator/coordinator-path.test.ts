import { describe, expect, it } from "vitest";
import { coordinatorIdFromPath } from "./coordinator-path";

describe("coordinatorIdFromPath", () => {
  it("reads the id from the Needs you and Queue routes", () => {
    expect(coordinatorIdFromPath("/workspaces/ws-1/coordinator/co-1")).toBe("co-1");
    expect(coordinatorIdFromPath("/workspaces/ws-1/coordinator/co-1/")).toBe("co-1");
    expect(coordinatorIdFromPath("/workspaces/ws-1/coordinator/co-1/queue")).toBe("co-1");
  });

  it("returns null for the generic coordinator route and every other path", () => {
    expect(coordinatorIdFromPath("/workspaces/ws-1/coordinator")).toBeNull();
    expect(coordinatorIdFromPath("/workspaces/ws-1/coordinator/co-1/settings")).toBeNull();
    expect(coordinatorIdFromPath("/workspaces/ws-1/tasks")).toBeNull();
    expect(coordinatorIdFromPath("/")).toBeNull();
  });
});
