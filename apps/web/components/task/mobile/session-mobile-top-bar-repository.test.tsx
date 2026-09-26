import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { pluginRegistry } from "@/lib/plugins/registry";
import { SessionMobileTopBar } from "./session-mobile-top-bar";

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(TEST_PROVIDER_PLUGIN_ID);
});

// Git metrics are not what this file is about; stub the two session-data hooks
// so the header renders standalone.
vi.mock("@/hooks/domains/session/use-session-git-status", () => ({
  useSessionGitStatus: () => ({ files: [] }),
  useSessionGitStatusByRepo: () => [],
}));

vi.mock("@/hooks/domains/session/use-session-commits", () => ({
  useSessionCommits: () => ({ commits: [] }),
}));

// Trailing controls the header composes but this file does not exercise.
vi.mock("@/components/task/port-forward-dialog", () => ({
  PortForwardButton: () => null,
}));

vi.mock("@/components/task/task-top-bar-plugin-actions", () => ({
  TaskTopBarPluginActions: () => null,
  useHasTaskTopBarPluginActions: () => false,
}));

vi.mock("@/components/gitlab/mr-topbar-button", () => ({
  MRTopbarButton: () => null,
}));

vi.mock("@/components/task/task-unarchive-button", () => ({
  TaskUnarchiveButton: ({ mobile }: { mobile?: boolean }) =>
    mobile ? <button data-testid="mobile-unarchive-button">Unarchive</button> : null,
}));

const REPOSITORY_TEST_ID = "mobile-task-repository";
const TEST_PROVIDER_PLUGIN_ID = "mobile-task-topbar-provider-test";
const TEST_PROVIDER_ID = "test_source_control";
const REMOTE_REPOSITORY_NAME = "agent-orchestrator";
const REMOTE_REPOSITORY_FULL_NAME = `owner/${REMOTE_REPOSITORY_NAME}`;
const REMOTE_REPOSITORY_URL = `https://github.com/${REMOTE_REPOSITORY_FULL_NAME}`;
const REPOSITORY_LINK_TEST_ID = "mobile-task-repository-link";

function TestProviderIcon({ className }: { className?: string }) {
  return <svg className={className} data-testid="registered-mobile-provider-icon" />;
}

function registerTestRepositoryProvider() {
  pluginRegistry.forPlugin(TEST_PROVIDER_PLUGIN_ID).registerRepositoryProvider({
    id: TEST_PROVIDER_ID,
    label: "Test Forge",
    icon: TestProviderIcon,
    listRepositories: async () => [],
    listBranches: async () => [],
    inspectURL: async () => null,
  });
}

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
            onTaskPickerClick={vi.fn()}
            taskPickerOpen={false}
            {...props}
          />
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
}

describe("SessionMobileTopBar repository", () => {
  // @covers AC-UI-REMOTE-REPO-TOPBAR-001.1, AC-UI-REMOTE-REPO-TOPBAR-001.2,
  // AC-UI-REMOTE-REPO-TOPBAR-001.5
  it("renders one remote repository before the task picker as its own external link", () => {
    renderTopBar({
      repositoryLabel: REMOTE_REPOSITORY_FULL_NAME,
      topbarRepository: {
        displayName: REMOTE_REPOSITORY_NAME,
        fullName: REMOTE_REPOSITORY_FULL_NAME,
        provider: "github",
        browserUrl: REMOTE_REPOSITORY_URL,
      },
    });

    const link = screen.getByTestId(REPOSITORY_LINK_TEST_ID);
    const picker = screen.getByTestId("mobile-task-picker-trigger");
    expect(link.tagName).toBe("A");
    expect(link.getAttribute("href")).toBe(REMOTE_REPOSITORY_URL);
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toBe("noopener noreferrer");
    expect(picker.contains(link)).toBe(false);
    expect(link.compareDocumentPosition(picker) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.queryByTestId(REPOSITORY_TEST_ID)).toBeNull();
  });

  // @covers AC-UI-REMOTE-REPO-TOPBAR-001.4
  it("keeps the remote repository identity visible without rendering an unsafe link", () => {
    renderTopBar({
      topbarRepository: {
        displayName: REMOTE_REPOSITORY_NAME,
        fullName: REMOTE_REPOSITORY_FULL_NAME,
        provider: "github",
        browserUrl: null,
      },
    });

    expect(screen.getByTestId("mobile-task-repository-label").textContent).toContain(
      REMOTE_REPOSITORY_NAME,
    );
    expect(screen.queryByTestId(REPOSITORY_LINK_TEST_ID)).toBeNull();
  });

  it("refreshes the provider label and icon after plugin registration changes", () => {
    renderTopBar({
      topbarRepository: {
        displayName: REMOTE_REPOSITORY_NAME,
        fullName: REMOTE_REPOSITORY_FULL_NAME,
        provider: TEST_PROVIDER_ID,
        browserUrl: `https://code.example.test/${REMOTE_REPOSITORY_FULL_NAME}`,
      },
    });

    const repositoryLink = screen.getByTestId(REPOSITORY_LINK_TEST_ID);
    expect(repositoryLink.getAttribute("aria-label")).toBe(
      `Test Source Control repository ${REMOTE_REPOSITORY_FULL_NAME}`,
    );
    expect(screen.queryByTestId("registered-mobile-provider-icon")).toBeNull();

    act(() => registerTestRepositoryProvider());

    expect(screen.getByTestId(REPOSITORY_LINK_TEST_ID).getAttribute("aria-label")).toBe(
      `Test Forge repository ${REMOTE_REPOSITORY_FULL_NAME}`,
    );
    expect(screen.getByTestId("registered-mobile-provider-icon")).toBeTruthy();

    act(() => pluginRegistry.unregisterPlugin(TEST_PROVIDER_PLUGIN_ID));

    expect(screen.getByTestId(REPOSITORY_LINK_TEST_ID).getAttribute("aria-label")).toBe(
      `Test Source Control repository ${REMOTE_REPOSITORY_FULL_NAME}`,
    );
    expect(screen.queryByTestId("registered-mobile-provider-icon")).toBeNull();
  });

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

  it("renders the existing unarchive action in the mobile top bar", () => {
    renderTopBar({ isArchived: true });

    expect(screen.getByTestId("mobile-unarchive-button")).toBeTruthy();
  });
});
