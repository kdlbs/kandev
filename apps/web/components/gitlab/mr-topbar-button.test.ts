import { createElement } from "react";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import {
  MRTopbarButton,
  mrTriggerClass,
  openDesktopMRReview,
  openMobileMRReview,
} from "./mr-topbar-button";
import type { TaskMR } from "@/lib/types/gitlab";

const gitlabMocks = vi.hoisted(() => ({ mrs: [] as TaskMR[] }));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      tasks: { activeTaskId: "task-1" },
      workspaces: { activeId: "workspace-1" },
      repositories: { itemsByWorkspaceId: { "workspace-1": [] } },
      removeTaskMR: vi.fn(),
    }),
}));

vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({
  useGitLabAvailable: () => true,
  useTaskMRs: () => gitlabMocks.mrs,
  useWorkspaceMRs: vi.fn(),
}));

vi.mock("@/hooks/domains/kanban/use-task-by-id", () => ({
  useTaskById: () => ({ id: "task-1", repositories: [] }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("./task-mr-link-dialog", () => ({ TaskMRLinkDialog: () => null }));

describe("mrTriggerClass", () => {
  it("does not render a generic link control for an unlinked task", () => {
    gitlabMocks.mrs = [];
    render(createElement(MRTopbarButton));

    expect(screen.queryByRole("button", { name: "Link GitLab merge request" })).toBeNull();
  });

  it("keeps the linked merge request status control", () => {
    gitlabMocks.mrs = [
      {
        id: "association-1",
        mr_iid: 81,
        state: "opened",
        project_path: "group/project",
        mr_url: "https://gitlab.example/group/project/-/merge_requests/81",
      } as TaskMR,
    ];
    render(createElement(MRTopbarButton));

    const trigger = screen.getByTestId("mr-topbar-button");
    expect(trigger.getAttribute("data-mr-iid")).toBe("81");
  });

  it("uses a 44px trigger on mobile", () => {
    expect(mrTriggerClass(true, true)).toContain("h-11");
    expect(mrTriggerClass(true, true)).toContain("w-11");
  });

  it("opens the exact selected MR instead of the first linked MR", () => {
    const setReview = vi.fn();
    openMobileMRReview(setReview, "session-1", {
      host: "https://gitlab.example",
      project_path: "group/b",
      mr_iid: 22,
    } as TaskMR);
    expect(setReview).toHaveBeenCalledWith("session-1", "https://gitlab.example|group/b|22");
  });

  it("confirms desktop MR focus after dockview finishes its layout work", () => {
    const addMRPanel = vi.fn();
    const scheduled: FrameRequestCallback[] = [];
    const schedule = vi.fn((callback: FrameRequestCallback) => {
      scheduled.push(callback);
      return scheduled.length;
    });
    const mr = {
      host: "https://gitlab.example",
      project_path: "group/b",
      mr_iid: 22,
    } as TaskMR;

    openDesktopMRReview(addMRPanel, "session-1", mr, schedule);
    expect(addMRPanel).toHaveBeenCalledTimes(1);
    scheduled.shift()?.(0);
    expect(addMRPanel).toHaveBeenCalledTimes(1);
    scheduled.shift()?.(0);
    expect(addMRPanel).toHaveBeenLastCalledWith("https://gitlab.example|group/b|22", "session-1");
    expect(addMRPanel).toHaveBeenCalledTimes(2);
  });
});
