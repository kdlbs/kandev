import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { PRTaskIcon } from "./pr-task-icon";
import type { TaskPRInfo } from "./pr-task-automation";

const listTaskPRsMock = vi.hoisted(() => vi.fn());
const getTaskCIAutomationOptionsMock = vi.hoisted(() => vi.fn());
const responsiveMock = vi.hoisted(() => ({ isFinePointer: true }));
const TASK_ID = "task-1";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    breakpoint: responsiveMock.isFinePointer ? "desktop" : "mobile",
    isMobile: !responsiveMock.isFinePointer,
    isTablet: false,
    isDesktop: responsiveMock.isFinePointer,
    isCompactDesktop: false,
    isFullDesktop: responsiveMock.isFinePointer,
    isFinePointer: responsiveMock.isFinePointer,
    usesDesktopWorkbench: responsiveMock.isFinePointer,
  }),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  listTaskPRs: listTaskPRsMock,
  getTaskCIAutomationOptions: getTaskCIAutomationOptionsMock,
}));

const compactCIInfo = {
  number: 42,
  state: "open",
  ciState: "in_progress",
  ciPRNumber: 42,
  ciRepository: "acme/widget",
} as TaskPRInfo;

function renderCompactCI() {
  render(
    <StateProvider initialState={{ workspaces: { items: [], activeId: "workspace-1" } }}>
      <TooltipProvider>
        <PRTaskIcon taskId={TASK_ID} prInfo={compactCIInfo} />
      </TooltipProvider>
    </StateProvider>,
  );
  return screen.getByTestId(`pr-task-icon-${TASK_ID}`);
}

beforeEach(() => {
  responsiveMock.isFinePointer = true;
  listTaskPRsMock.mockReset().mockRejectedValue(new Error("offline"));
  getTaskCIAutomationOptionsMock.mockReset().mockResolvedValue(undefined);
});

afterEach(() => cleanup());

describe("PRTaskIcon compact CI disclosure without full records", () => {
  it("keeps the known CI reason and selected identity beside an unavailable hydration result", async () => {
    const icon = renderCompactCI();
    const matches = vi.spyOn(icon, "matches").mockReturnValue(true);
    fireEvent.focus(icon);
    matches.mockRestore();

    expect((await screen.findByTestId("pr-task-status-ci-ci-state")).textContent).toContain(
      "In progress",
    );
    expect(screen.getByTestId("pr-task-status-ci-ci-state-detail").textContent).toContain(
      "acme/widget PR #42",
    );
    expect(await screen.findByTestId("pr-task-tooltip-unavailable")).toBeTruthy();
  });

  it("keeps the known CI reason beside a loading hydration result", async () => {
    listTaskPRsMock.mockReset().mockReturnValue(new Promise(() => {}));
    const icon = renderCompactCI();
    const matches = vi.spyOn(icon, "matches").mockReturnValue(true);
    fireEvent.focus(icon);
    matches.mockRestore();

    expect((await screen.findByTestId("pr-task-status-ci-ci-state")).textContent).toContain(
      "In progress",
    );
    expect(screen.getByTestId("pr-task-tooltip-loading")).toBeTruthy();
  });

  it("keeps the known CI reason in the touch drawer", async () => {
    responsiveMock.isFinePointer = false;
    const icon = renderCompactCI();
    fireEvent.click(icon);

    const drawer = await screen.findByTestId(`pr-task-automation-drawer-${TASK_ID}`);
    expect(drawer.textContent).toContain("In progress");
    expect(drawer.textContent).toContain("acme/widget PR #42");
    expect(await screen.findByTestId("pr-task-tooltip-unavailable")).toBeTruthy();
  });
});
