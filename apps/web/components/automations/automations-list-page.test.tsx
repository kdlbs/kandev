import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import type { Automation } from "@/lib/types/automation";

const mockUseAutomations = vi.fn();
vi.mock("@/hooks/domains/settings/use-automations", () => ({
  useAutomations: (...args: unknown[]) => mockUseAutomations(...args),
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

import { AutomationsListPage } from "./automations-list-page";

const NOTICE = "automation-board-move-notice";

function mkAutomation(id: string, overrides: Partial<Automation> = {}): Automation {
  return {
    id,
    workspace_id: "ws-1",
    name: id,
    description: "",
    workflow_id: "",
    workflow_step_id: "",
    agent_profile_id: "",
    executor_profile_id: "",
    repository_ids: [],
    prompt: "",
    task_title_template: "",
    enabled: true,
    max_concurrent_runs: 1,
    last_triggered_at: null,
    created_at: "2026-05-22T00:00:00Z",
    updated_at: "2026-05-22T00:00:00Z",
    triggers: [],
    ...overrides,
  };
}

function renderPage(items: Automation[], workspaceId = "ws-1", remove = vi.fn()) {
  mockUseAutomations.mockReturnValue({
    items,
    loading: false,
    enable: vi.fn(),
    disable: vi.fn(),
    trigger: vi.fn(),
    remove,
  });
  return render(
    <SettingsSaveProvider>
      <TooltipProvider>
        <AutomationsListPage workspaceId={workspaceId} />
      </TooltipProvider>
    </SettingsSaveProvider>,
  );
}

describe("AutomationsListPage board-move notice", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  // The upgrade takes the kanban card away from every automation stored in the
  // `task` mode, which was the default. Saying so on the surface the runs
  // moved to is the whole migration strategy.
  it("tells a workspace whose automations used to produce board cards", () => {
    renderPage([mkAutomation("a-modern"), mkAutomation("a-legacy", { legacy_board_card: true })]);

    const notice = screen.getByTestId(NOTICE);
    expect(notice.textContent).toContain("no longer appear on the board");
  });

  // One-time means one time. A dismissal that a reload undoes is a nag.
  it("stays dismissed across a remount", () => {
    const items = [mkAutomation("a-legacy", { legacy_board_card: true })];
    renderPage(items);

    fireEvent.click(screen.getByTestId(`${NOTICE}-dismiss`));
    expect(screen.queryByTestId(NOTICE)).toBeNull();

    cleanup();
    renderPage(items);
    expect(screen.queryByTestId(NOTICE)).toBeNull();
  });

  // Dismissal is per workspace: a second workspace's cards vanished too, and
  // reading the news for the first one is not being told about the second.
  it("still warns a different workspace after one is dismissed", () => {
    const legacy = [mkAutomation("a-legacy", { legacy_board_card: true })];
    renderPage(legacy, "ws-1");
    fireEvent.click(screen.getByTestId(`${NOTICE}-dismiss`));
    cleanup();

    renderPage([mkAutomation("b-legacy", { legacy_board_card: true })], "ws-2");
    expect(screen.getByTestId(NOTICE)).toBeTruthy();
  });

  // An install that only ever used `run` mode lost nothing — its runs were
  // already hidden. Telling it that something changed would be false.
  it("says nothing to an install whose automations never made board cards", () => {
    renderPage([mkAutomation("a-run-mode"), mkAutomation("b-run-mode")]);

    expect(screen.queryByTestId(NOTICE)).toBeNull();
  });

  it("keeps the delete confirmation open when deletion fails", async () => {
    const remove = vi.fn().mockRejectedValue(new Error("temporary failure"));
    renderPage([mkAutomation("automation-a", { name: "Delete me" })], "ws-1", remove);

    fireEvent.click(screen.getByTestId("automation-delete-button-automation-a"));
    fireEvent.click(screen.getByTestId("automation-delete-confirm"));

    await waitFor(() => expect(remove).toHaveBeenCalledWith("automation-a"));
    expect(screen.getByRole("alertdialog")).toBeTruthy();
    expect(screen.getByText("Delete me")).toBeTruthy();
  });
});
