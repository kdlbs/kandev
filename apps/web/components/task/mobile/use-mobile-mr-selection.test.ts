import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskMR } from "@/lib/types/gitlab";

const mocks = vi.hoisted(() => ({
  mrs: [] as TaskMR[],
  setMobileSessionReview: vi.fn(),
  state: {
    workspaces: { activeId: "workspace-1" },
    taskMRs: { byWorkspaceId: { "workspace-1": {} } },
    mobileSession: { reviewMRKeyBySessionId: {} as Record<string, string> },
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: typeof mocks.state & { setMobileSessionReview: typeof vi.fn }) => unknown,
  ) => selector({ ...mocks.state, setMobileSessionReview: mocks.setMobileSessionReview }),
}));

vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({
  useTaskMRs: () => mocks.mrs,
}));

vi.mock("@/components/gitlab/mr-detail-panel", () => ({
  mrTaskKey: (mr: Pick<TaskMR, "host" | "project_path" | "mr_iid">) =>
    `${mr.host}|${mr.project_path}|${mr.mr_iid}`,
  selectExplicitPanelMR: (mrs: TaskMR[], key: string | null) =>
    mrs.find((mr) => `${mr.host}|${mr.project_path}|${mr.mr_iid}` === key) ?? null,
}));

import { useMobileMRSelection } from "./use-mobile-mr-selection";

const primaryMR = {
  host: "https://gitlab.example",
  project_path: "group/project",
  mr_iid: 42,
} as TaskMR;

describe("useMobileMRSelection", () => {
  beforeEach(() => {
    mocks.mrs = [];
    mocks.setMobileSessionReview.mockReset();
    mocks.state.mobileSession.reviewMRKeyBySessionId = {};
  });

  it("clears an invalid persisted review selection once review sources resolve", () => {
    mocks.state.mobileSession.reviewMRKeyBySessionId = { "session-1": "missing" };
    const changePanel = vi.fn();

    renderHook(() => useMobileMRSelection("task-1", "session-1", "review", changePanel, false));

    expect(mocks.setMobileSessionReview).toHaveBeenCalledWith("session-1", null);
  });

  it("selects the primary GitLab merge request when opening Review", () => {
    mocks.mrs = [primaryMR];
    const changePanel = vi.fn();
    const { result } = renderHook(() =>
      useMobileMRSelection("task-1", "session-1", "chat", changePanel, false),
    );

    act(() => result.current.handlePanelChange("review"));

    expect(mocks.setMobileSessionReview).toHaveBeenCalledWith(
      "session-1",
      "https://gitlab.example|group/project|42",
    );
    expect(changePanel).toHaveBeenCalledWith("review");
  });
});
