import { describe, expect, it } from "vitest";
import {
  linkToCoordinatorAdd,
  linkToCoordinatorNeedsYou,
  linkToCoordinatorQueue,
  linkToCoordinatorSettings,
  linkToCoordinatorSettingsList,
} from "./links";

describe("linkToCoordinatorNeedsYou", () => {
  it("builds the Needs you path", () => {
    expect(linkToCoordinatorNeedsYou("ws-1", "co-1")).toBe("/workspaces/ws-1/coordinator/co-1");
  });

  it("encodes ids", () => {
    expect(linkToCoordinatorNeedsYou("ws 1", "co/1")).toBe("/workspaces/ws%201/coordinator/co%2F1");
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
