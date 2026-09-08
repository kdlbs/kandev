import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { JiraIssueWatch } from "@/lib/types/jira";
import { JiraIssueWatchTable } from "./jira-issue-watch-table";

const responsive = vi.hoisted(() => ({
  isFinePointer: true,
  usesDesktopWorkbench: true,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ workspaces: { items: [{ id: "ws-1", name: "Workspace One" }] } }),
}));

function watch(overrides: Partial<JiraIssueWatch> = {}): JiraIssueWatch {
  return {
    id: "jira-1",
    workspaceId: "ws-1",
    jql: "project = KANDEV",
    workflowId: "workflow-1",
    workflowStepId: "step-1",
    repositoryId: "repo-1",
    baseBranch: "main",
    agentProfileId: "agent-1",
    executorProfileId: "executor-1",
    prompt: "Create task",
    enabled: true,
    pollIntervalSeconds: 300,
    lastPolledAt: null,
    ...overrides,
  } as JiraIssueWatch;
}

function renderTable(onDelete = vi.fn()) {
  return render(
    <TooltipProvider>
      <JiraIssueWatchTable
        watches={[watch()]}
        dirtyIds={new Set()}
        showWorkspace
        onEdit={vi.fn()}
        onDelete={onDelete}
        onTrigger={vi.fn()}
        onReset={vi.fn()}
        onToggleEnabled={vi.fn()}
      />
    </TooltipProvider>,
  );
}

afterEach(() => {
  cleanup();
  responsive.isFinePointer = true;
  responsive.usesDesktopWorkbench = true;
  Object.defineProperty(window, "confirm", { configurable: true, value: undefined });
});

function installNativeConfirm() {
  const nativeConfirm = vi.fn();
  Object.defineProperty(window, "confirm", {
    configurable: true,
    value: nativeConfirm,
  });
  return nativeConfirm;
}

describe("JiraIssueWatchTable delete confirmation", () => {
  it("confirms beside the delete action and never calls browser confirm", async () => {
    const onDelete = vi.fn();
    const nativeConfirm = installNativeConfirm();
    renderTable(onDelete);

    fireEvent.click(screen.getByTestId("jira-watch-delete-jira-1"));
    const confirmation = await screen.findByRole("dialog", {
      name: "Delete this JIRA watcher?",
    });
    expect(nativeConfirm).not.toHaveBeenCalled();

    fireEvent.click(within(confirmation).getByRole("button", { name: "Cancel" }));
    expect(onDelete).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    fireEvent.click(screen.getByTestId("jira-watch-delete-jira-1"));
    fireEvent.click(
      within(await screen.findByRole("dialog", { name: "Delete this JIRA watcher?" })).getByRole(
        "button",
        { name: "Delete" },
      ),
    );
    await waitFor(() => expect(onDelete).toHaveBeenCalledWith("jira-1"));
  });

  it("preserves workspace identity in install-wide mobile cards", () => {
    responsive.usesDesktopWorkbench = false;
    renderTable();

    expect(
      within(screen.getByTestId("jira-watch-mobile-row-jira-1")).getByText("Workspace One"),
    ).toBeTruthy();
  });

  it("keeps coarse-pointer confirmation inline with 44px actions", async () => {
    responsive.isFinePointer = false;
    const onDelete = vi.fn();
    renderTable(onDelete);

    fireEvent.click(screen.getByTestId("jira-watch-delete-jira-1"));
    const confirmation = await screen.findByRole("group", {
      name: "Delete this JIRA watcher?",
    });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(within(confirmation).getByRole("button", { name: "Cancel" }).className).toContain(
      "h-11",
    );
    expect(within(confirmation).getByRole("button", { name: "Delete" }).className).toContain(
      "min-w-11",
    );
    fireEvent.click(within(confirmation).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(onDelete).toHaveBeenCalledWith("jira-1"));
  });
});
