import { describe, expect, it } from "vitest";
import {
  linkToCoordinator,
  linkToCoordinatorAdd,
  linkToCoordinatorNeedsYou,
  linkToCoordinatorNeedsYouForm,
  linkToCoordinatorQueue,
  linkToCoordinatorSettings,
  linkToCoordinatorSettingsList,
} from "./links";

describe("linkToCoordinator", () => {
  it("builds the generic coordinator path", () => {
    expect(linkToCoordinator("ws-1")).toBe("/workspaces/ws-1/coordinator");
  });

  it("encodes the workspace id", () => {
    expect(linkToCoordinator("ws/1")).toBe("/workspaces/ws%2F1/coordinator");
  });
});

describe("linkToCoordinatorNeedsYou", () => {
  it("builds the Needs you path", () => {
    expect(linkToCoordinatorNeedsYou("ws-1", "co-1")).toBe("/workspaces/ws-1/coordinator/co-1");
  });

  it("encodes ids", () => {
    expect(linkToCoordinatorNeedsYou("ws 1", "co/1")).toBe("/workspaces/ws%201/coordinator/co%2F1");
  });
});

describe("linkToCoordinatorNeedsYouForm", () => {
  it("builds the Needs you path with the proposal and form query params", () => {
    expect(linkToCoordinatorNeedsYouForm("ws-1", "co-1", "p-1", "edit")).toBe(
      "/workspaces/ws-1/coordinator/co-1?proposal=p-1&form=edit",
    );
  });

  it("encodes the proposal id", () => {
    expect(linkToCoordinatorNeedsYouForm("ws-1", "co-1", "p/1", "reject")).toBe(
      "/workspaces/ws-1/coordinator/co-1?proposal=p%2F1&form=reject",
    );
  });
});

describe("linkToCoordinatorQueue", () => {
  it("builds the Queue path with no group", () => {
    expect(linkToCoordinatorQueue("ws-1", "co-1")).toBe("/workspaces/ws-1/coordinator/co-1/queue");
  });

  it("appends the group query param", () => {
    expect(linkToCoordinatorQueue("ws-1", "co-1", "working")).toBe(
      "/workspaces/ws-1/coordinator/co-1/queue?group=working",
    );
  });
});

describe("linkToCoordinatorSettingsList", () => {
  it("builds the settings list path", () => {
    expect(linkToCoordinatorSettingsList("ws-1")).toBe("/settings/workspaces/ws-1/coordinators");
  });
});

describe("linkToCoordinatorSettings", () => {
  it("builds one coordinator's settings path", () => {
    expect(linkToCoordinatorSettings("ws-1", "co-1")).toBe(
      "/settings/workspaces/ws-1/coordinators/co-1",
    );
  });
});

describe("linkToCoordinatorAdd", () => {
  it("builds the add-coordinator path", () => {
    expect(linkToCoordinatorAdd("ws-1")).toBe("/settings/workspaces/ws-1/coordinators/new");
  });
});
