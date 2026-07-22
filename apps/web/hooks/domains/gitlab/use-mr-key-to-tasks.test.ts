import { describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { createElement, type ReactNode } from "react";
import type { TaskMR } from "@/lib/types/gitlab";

vi.mock("@/lib/api/domains/gitlab-api", () => ({
  listWorkspaceTaskMRs: () => new Promise(() => {}),
}));

import { mrKey, useMRKeyToTasks } from "./use-mr-key-to-tasks";

const PROJECT_PATH = "group/project";
const GITLAB_HOST = "https://gitlab.com";

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

function makeMR(overrides: Partial<TaskMR>): TaskMR {
  return {
    id: "association-1",
    task_id: "task-1",
    host: GITLAB_HOST,
    project_path: PROJECT_PATH,
    mr_iid: 7,
    mr_url: "https://gitlab.com/group/project/-/merge_requests/7",
    mr_title: "MR",
    head_branch: "feature",
    base_branch: "main",
    author_username: "alice",
    state: "opened",
    approval_state: "",
    pipeline_state: "",
    merge_status: "",
    draft: false,
    approval_count: 0,
    required_approvals: 0,
    pipeline_jobs_total: 0,
    pipeline_jobs_pass: 0,
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

describe("useMRKeyToTasks", () => {
  it("groups multiple tasks by project path and MR IID", () => {
    const { result } = renderHook(
      () => {
        const map = useMRKeyToTasks("ws-1");
        const setTaskMRs = useAppStore((state) => state.setTaskMRs);
        return { map, setTaskMRs };
      },
      { wrapper },
    );
    act(() => {
      result.current.setTaskMRs("ws-1", {
        "task-1": [makeMR({})],
        "task-2": [makeMR({ id: "association-2", task_id: "task-2" })],
      });
    });
    expect(
      result.current.map.get(mrKey(GITLAB_HOST, PROJECT_PATH, 7))?.map((row) => row.task_id),
    ).toEqual(["task-1", "task-2"]);
  });

  it("separates the same full project path and IID across GitLab hosts", () => {
    const { result } = renderHook(
      () => {
        const map = useMRKeyToTasks("ws-1");
        const setTaskMRs = useAppStore((state) => state.setTaskMRs);
        return { map, setTaskMRs };
      },
      { wrapper },
    );
    act(() => {
      result.current.setTaskMRs("ws-1", {
        "task-public": [makeMR({ task_id: "task-public", host: GITLAB_HOST })],
        "task-private": [
          makeMR({ id: "private", task_id: "task-private", host: "https://gitlab.internal" }),
        ],
      });
    });
    expect(
      result.current.map
        .get(mrKey("https://gitlab.com/", PROJECT_PATH, 7))
        ?.map((row) => row.task_id),
    ).toEqual(["task-public"]);
    expect(
      result.current.map
        .get(mrKey("https://gitlab.internal", PROJECT_PATH, 7))
        ?.map((row) => row.task_id),
    ).toEqual(["task-private"]);
  });

  it("indexes only associations owned by the requested workspace", () => {
    const { result } = renderHook(
      () => {
        const map = useMRKeyToTasks("ws-b");
        const setTaskMRs = useAppStore((state) => state.setTaskMRs);
        return { map, setTaskMRs };
      },
      { wrapper },
    );
    act(() => {
      result.current.setTaskMRs("ws-b", {
        "task-b": [makeMR({ id: "b", task_id: "task-b" })],
      });
      result.current.setTaskMRs("ws-a", {
        "task-a": [makeMR({ id: "a", task_id: "task-a" })],
      });
    });

    expect(
      result.current.map.get(mrKey(GITLAB_HOST, PROJECT_PATH, 7))?.map((row) => row.task_id),
    ).toEqual(["task-b"]);
  });
});
