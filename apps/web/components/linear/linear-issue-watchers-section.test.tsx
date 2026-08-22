import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { i18n } from "@/lib/i18n";
import type { LinearIssueWatch } from "@/lib/types/linear";
import { LinearIssueWatchTable } from "./linear-issue-watch-table";

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

afterEach(async () => {
  cleanup();
  await i18n.changeLanguage("en");
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

describe("LinearIssueWatchTable delete confirmation", () => {
  it("uses anchored localized confirmation instead of browser confirm", async () => {
    await i18n.changeLanguage("pseudo");
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
    expect(await screen.findByRole("dialog", { name: "Ďēĺēţē ţĥĩś Ĺĩńēàŕ ŵàţćĥēŕ?" })).toBeTruthy();
    expect(nativeConfirm).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Ćàńćēĺ" }));
    expect(onDelete).not.toHaveBeenCalled();
  });

  it("preserves workspace identity in install-wide mobile cards", () => {
    responsive.usesDesktopWorkbench = false;
    render(
      <TooltipProvider>
        <LinearIssueWatchTable
          watches={[watch()]}
          dirtyIds={new Set()}
          showWorkspace
          onEdit={vi.fn()}
          onDelete={vi.fn()}
          onTrigger={vi.fn()}
          onReset={vi.fn()}
          onToggleEnabled={vi.fn()}
        />
      </TooltipProvider>,
    );

    expect(
      within(screen.getByTestId("linear-watch-mobile-row-linear-1")).getByText("Workspace One"),
    ).toBeTruthy();
  });

  it("uses touch-sized inline actions for coarse pointers", async () => {
    await i18n.changeLanguage("pseudo");
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
      name: "Ďēĺēţē ţĥĩś Ĺĩńēàŕ ŵàţćĥēŕ?",
    });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByRole("button", { name: "Ćàńćēĺ" }).className).toContain("h-11");
    fireEvent.click(screen.getByRole("button", { name: "Ďēĺēţē" }));
    await waitFor(() => expect(onDelete).toHaveBeenCalledWith("linear-1"));
    await waitFor(() => expect(confirmation.isConnected).toBe(false));
  });
});
