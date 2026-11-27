import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Automation, AutomationRun } from "@/lib/types/automation";
import { emitAutomationRunRecorded } from "@/lib/automation-run-recorded";

const AUTOMATION_ID = "automation-1";
const WORKSPACE_ID = "workspace-1";

const mocks = vi.hoisted(() => ({
  getAutomation: vi.fn(),
  getAutomationSummary: vi.fn(),
  listAutomationRuns: vi.fn(),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false, isFinePointer: true }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { workspaces: { activeId: string } }) => unknown) =>
    selector({ workspaces: { activeId: WORKSPACE_ID } }),
}));
vi.mock("@/lib/api/domains/automation-api", () => ({
  getAutomation: mocks.getAutomation,
  getAutomationSummary: mocks.getAutomationSummary,
  listAutomationRuns: mocks.listAutomationRuns,
  triggerAutomation: vi.fn(),
  updateTrigger: vi.fn(),
  stopAutomationRun: vi.fn(),
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => `/automations/${AUTOMATION_ID}`,
}));
vi.mock("sonner", () => ({ toast: { info: vi.fn(), success: vi.fn(), error: vi.fn() } }));
vi.mock("./run-transcript", () => ({
  RunTranscript: ({ sessionId }: { sessionId: string }) => (
    <div data-testid="run-transcript" data-session-id={sessionId} />
  ),
}));

import { AutomationDetailPage } from "./automation-detail-page";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("AutomationDetailPage live runs", () => {
  // @covers AC-OFFICE-AUTOMATION-RUNS-001.23
  it("shows a run recorded while nothing was open, without a reload", async () => {
    mocks.getAutomation.mockResolvedValue({
      id: AUTOMATION_ID,
      workspace_id: WORKSPACE_ID,
      name: "PR reviewer",
      enabled: true,
      max_concurrent_runs: 1,
      triggers: [],
    } as unknown as Automation);
    mocks.getAutomationSummary.mockResolvedValue({ automation_id: AUTOMATION_ID, open_runs: 0 });
    mocks.listAutomationRuns.mockResolvedValue([]);
    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);
    await screen.findAllByText("PR reviewer");
    expect(screen.queryByTestId("run-transcript")).toBeNull();

    mocks.listAutomationRuns.mockResolvedValue([
      {
        id: "run-pr",
        automation_id: AUTOMATION_ID,
        trigger_type: "github_pr",
        session_id: "session-pr",
        status: "succeeded",
        trigger_data: {},
        created_at: "2026-11-24T22:00:00Z",
      } as unknown as AutomationRun,
    ]);
    // Re-announce until the page has subscribed; the interval outlasts the
    // debounce so each announcement can complete a refresh.
    await vi.waitFor(
      () => {
        emitAutomationRunRecorded({
          workspace_id: WORKSPACE_ID,
          automation_id: AUTOMATION_ID,
          run_id: "run-pr",
          status: "succeeded",
        });
        expect(mocks.listAutomationRuns.mock.calls.length).toBeGreaterThanOrEqual(2);
      },
      { timeout: 10_000, interval: 500 },
    );
    const transcript = await screen.findByTestId("run-transcript", {}, { timeout: 10_000 });
    expect(transcript.getAttribute("data-session-id")).toBe("session-pr");
  });
});
