import { describe, expect, it } from "vitest";
import { officeSetupHref } from "./office-setup";

describe("office setup follows Kanban setup", () => {
  it("takes unfinished onboarding to Kanban first", () => {
    expect(officeSetupHref(false, "ws")).toBe("/?home=overview");
  });
  it("sets up personas inside the existing workspace", () => {
    expect(officeSetupHref(true, "ws")).toBe("/settings/workspaces/ws/agents");
  });
  it("asks for workspace setup when none exists", () => {
    expect(officeSetupHref(true, null)).toBe("/settings/workspaces");
  });
});

it("switches between Office and board inside the same workspace", async () => {
  const { workspaceOfficeHref } = await import("./office-setup");
  expect(workspaceOfficeHref(true, "ws", true)).toBe("/?home=overview&workspaceId=ws");
  expect(workspaceOfficeHref(false, "ws", true)).toBe("/office?workspaceId=ws");
  expect(workspaceOfficeHref(false, "ws", false)).toBe("/settings/workspaces/ws/agents");
});
