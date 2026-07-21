import { describe, expect, it } from "vitest";
import {
  isOpenMRState,
  isTaskMRHostAllowed,
  mrTaskKey,
  selectExplicitPanelMR,
  selectPanelMR,
} from "./mr-detail-panel";
import type { TaskMR } from "@/lib/types/gitlab";

describe("mrTaskKey", () => {
  it("includes the normalized host and full subgroup path", () => {
    expect(
      mrTaskKey({
        host: "https://gitlab.acme.test/",
        project_path: "group/sub/project",
        mr_iid: 12,
      }),
    ).toBe("https://gitlab.acme.test|group/sub/project|12");
  });
});

describe("selectPanelMR", () => {
  const first = {
    id: "first",
    host: "https://gitlab.com",
    project_path: "a/b",
    mr_iid: 1,
  } as TaskMR;

  it("uses the first MR only for a legacy unkeyed panel", () => {
    expect(selectPanelMR([first], undefined)).toBe(first);
  });

  it("fails closed when an explicit mrKey is missing", () => {
    expect(selectPanelMR([first], "https://gitlab.com|other/repo|9")).toBeNull();
  });

  it("selects only an explicit mobile key and fails closed when it is unlinked", () => {
    const second = { ...first, id: "second", project_path: "group/b", mr_iid: 22 };
    expect(selectExplicitPanelMR([first, second], mrTaskKey(second))).toBe(second);
    expect(selectExplicitPanelMR([first], mrTaskKey(second))).toBeNull();
    expect(selectExplicitPanelMR([first, second], null)).toBeNull();
  });
});

describe("isTaskMRHostAllowed", () => {
  it("requires a matching configured origin", () => {
    expect(isTaskMRHostAllowed("https://gitlab.acme.test/", "https://gitlab.acme.test")).toBe(true);
    expect(isTaskMRHostAllowed("https://gitlab.com", "https://gitlab.acme.test")).toBe(false);
    expect(isTaskMRHostAllowed("https://gitlab.com", null)).toBe(false);
  });
});

describe("isOpenMRState", () => {
  it("accepts both GitLab open-state spellings and rejects terminal states", () => {
    expect(isOpenMRState("open")).toBe(true);
    expect(isOpenMRState("opened")).toBe(true);
    expect(isOpenMRState("closed")).toBe(false);
    expect(isOpenMRState("merged")).toBe(false);
  });
});
