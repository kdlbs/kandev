import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { SessionMobileTopBar } from "./session-mobile-top-bar";

afterEach(cleanup);

// The phone bar's git machinery is not what this file is about; stub the two
// hooks that reach for session state so the header renders standalone.
vi.mock("@/hooks/domains/session/use-session-git-status", () => ({
  useSessionGitStatus: () => ({ files: [] }),
  useSessionGitStatusByRepo: () => [],
}));

vi.mock("@/hooks/domains/session/use-session-commits", () => ({
  useSessionCommits: () => ({ commits: [] }),
}));

vi.mock("@/hooks/domains/session/use-remote-contribution-relation", () => ({
  useRemoteContributionRelation: () => ({
    relation: { action: "none", canPull: false, canReplaceRemote: false, canUseRemote: false },
  }),
}));

// Trailing controls the header composes but this file does not exercise.
vi.mock("@/components/task/port-forward-dialog", () => ({
  PortForwardButton: () => null,
}));

vi.mock("@/components/task/task-top-bar-plugin-actions", () => ({
  TaskTopBarPluginActions: () => null,
}));

vi.mock("@/components/gitlab/mr-topbar-button", () => ({
  MRTopbarButton: () => null,
}));

const REPOSITORY_TEST_ID = "mobile-task-repository";

function renderTopBar(props: Record<string, unknown> = {}) {
  return render(
    <StateProvider>
      <ToastProvider>
        <TooltipProvider>
          <SessionMobileTopBar
            taskId="task-1"
            workspaceId="ws-1"
            taskTitle="Pin the RDS engine version"
            sessionId="session-1"
            onMenuClick={vi.fn()}
            {...props}
          />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
}

describe("SessionMobileTopBar repository", () => {
  it("names the task's repository, so the phone header says which project this is", () => {
    renderTopBar({ repositoryLabel: "kdlbs/kandev" });

    expect(screen.getByTestId(REPOSITORY_TEST_ID).textContent).toBe("kdlbs/kandev");
  });

  it("keeps the full repository name reachable on hover when it is truncated", () => {
    renderTopBar({ repositoryLabel: "acme-platform/infra-terraform-modules" });

    expect(screen.getByTestId(REPOSITORY_TEST_ID).getAttribute("title")).toBe(
      "acme-platform/infra-terraform-modules",
    );
  });

  it("renders nothing when the task has no repository", () => {
    renderTopBar();

    expect(screen.queryByTestId(REPOSITORY_TEST_ID)).toBeNull();
  });
});
