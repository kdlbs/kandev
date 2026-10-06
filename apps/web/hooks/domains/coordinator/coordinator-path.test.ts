import { describe, expect, it } from "vitest";
import { coordinatorIdFromPath } from "./coordinator-path";

const VIEWS = ["", "/", "/queue", "/queue/"];
const VALID_IDENTITIES = [
  { workspace: "ws-1", coordinator: "co-1", id: "co-1" },
  { workspace: "ws%2Fone", coordinator: "co%2F1", id: "co/1" },
  { workspace: "ws%25", coordinator: "co%25", id: "co%" },
  { workspace: "ws%252Fone", coordinator: "co%252F1", id: "co%2F1" },
  { workspace: "ws%E6%97%A5", coordinator: "co%E6%97%A5", id: "co日" },
];

// @covers AC-COORDINATOR-COPILOT-004.12
// The helper owns decoding and grammar; the bridge tests own the native effect/store boundary.
describe("coordinatorIdFromPath", () => {
  it.each(VALID_IDENTITIES)("decodes valid identity $coordinator exactly once", (identity) => {
    for (const view of VIEWS) {
      expect(
        coordinatorIdFromPath(
          `/workspaces/${identity.workspace}/coordinator/${identity.coordinator}${view}`,
        ),
      ).toBe(identity.id);
    }
  });

  it.each(["%", "ws%ZZ", "%E0%A4%A"])("rejects undecodable workspace %s", (workspace) => {
    for (const view of VIEWS) {
      expect(coordinatorIdFromPath(`/workspaces/${workspace}/coordinator/co-1${view}`)).toBeNull();
    }
  });

  it.each(["%", "co%ZZ", "%E0%A4%A"])("rejects undecodable coordinator %s", (coordinator) => {
    for (const view of VIEWS) {
      expect(
        coordinatorIdFromPath(`/workspaces/ws-1/coordinator/${coordinator}${view}`),
      ).toBeNull();
    }
  });

  it.each([
    "/",
    "/workspaces/ws-1/tasks",
    "/workspaces/ws-1/coordinator",
    "/workspaces/ws-1/coordinator/",
    "/workspaces//coordinator/co-1",
    "/workspaces/ws-1/coordinator//queue",
    "/workspaces/ws-1/coordinator/co-1/settings",
    "/workspaces/ws-1/coordinator/co-1/queue/extra",
  ])("returns null for unsupported/default path %s", (pathname) => {
    expect(coordinatorIdFromPath(pathname)).toBeNull();
  });
});
