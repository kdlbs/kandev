import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { LinearIssueWatch } from "@/lib/types/linear";
import { LinearIssueWatchTable } from "./linear-issue-watch-table";

const responsive = vi.hoisted(() => ({ isFinePointer: true }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) => selector({ workspaces: { items: [] } }),
}));

function watch(overrides: Partial<LinearIssueWatch> = {}): LinearIssueWatch {
  return {
    id: "linear-1",
    workspaceId: "ws-1",
    filter: { teamKey: "ENG" },
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
  } as LinearIssueWatch;
}

afterEach(() => {
  cleanup();
  responsive.isFinePointer = true;
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

describe("LinearIssueWatchTable delete confirmation", () => {
  it("uses anchored localized confirmation instead of browser confirm", async () => {
    const onDelete = vi.fn();
    const nativeConfirm = installNativeConfirm();
    render(
      <TooltipProvider>
        <LinearIssueWatchTable
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

    fireEvent.click(screen.getByTestId("linear-watch-delete-linear-1"));
    expect(await screen.findByRole("dialog", { name: "Delete this Linear watcher?" })).toBeTruthy();
    expect(nativeConfirm).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onDelete).not.toHaveBeenCalled();
  });

  it("uses touch-sized inline actions for coarse pointers", async () => {
    responsive.isFinePointer = false;
    const onDelete = vi.fn();
    render(
      <TooltipProvider>
        <LinearIssueWatchTable
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

    fireEvent.click(screen.getByTestId("linear-watch-delete-linear-1"));
    const confirmation = await screen.findByRole("group", {
      name: "Delete this Linear watcher?",
    });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByRole("button", { name: "Cancel" }).className).toContain("h-11");
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(onDelete).toHaveBeenCalledWith("linear-1"));
    expect(confirmation).toBeTruthy();
  });
});
