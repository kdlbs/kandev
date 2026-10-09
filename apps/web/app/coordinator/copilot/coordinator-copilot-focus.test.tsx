import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { OpenSequenceState } from "@/hooks/domains/coordinator/use-copilot-open-sequence";

const useCoordinatorCopilot = vi.hoisted(() => vi.fn());
const bodyProps = vi.hoisted(() => ({ current: undefined as Record<string, unknown> | undefined }));

vi.mock("./use-coordinator-copilot", () => ({ useCoordinatorCopilot }));

// The panel is reduced to "main + children while open" so the test isolates the
// launcher focus-return wiring in CoordinatorCopilot from layout and breakpoints.
vi.mock("@/components/right-side-panel", () => ({
  RightSidePanel: (props: { open: boolean; main: ReactNode; children: ReactNode }) => (
    <>
      {props.main}
      {props.open ? props.children : null}
    </>
  ),
}));

vi.mock("./coordinator-copilot-body", () => ({
  CoordinatorCopilotBody: (props: Record<string, unknown>) => {
    bodyProps.current = props;
    return null;
  },
}));

import { CoordinatorCopilot } from "./coordinator-copilot";

const WORKSPACE_ID = "ws-1";
const LAUNCHER_ID = "coordinator-copilot-launcher";
const COORDINATOR_ID = "coord-1";
const COORDINATOR_NAME = "Backend coordinator";

function mockController(overrides: Partial<ReturnType<typeof useCoordinatorCopilot>> = {}) {
  useCoordinatorCopilot.mockReturnValue({
    enabled: true,
    open: true,
    launcher: { coordinator: null, loading: false, busy: false, gone: false },
    openSequence: { state: { kind: "idle" } as OpenSequenceState, open: vi.fn(), retry: vi.fn() },
    routeSession: null,
    chip: null,
    pendingDraft: undefined,
    askKey: 0,
    handleOpenChange: vi.fn(),
    removeChip: vi.fn(),
    suggest: vi.fn(),
    ...overrides,
  });
}

function tree() {
  return (
    <TooltipProvider delayDuration={0}>
      <CoordinatorCopilot
        workspaceId={WORKSPACE_ID}
        coordinatorId={COORDINATOR_ID}
        coordinatorName={COORDINATOR_NAME}
        canManage
      >
        <div data-testid="screen" />
      </CoordinatorCopilot>
    </TooltipProvider>
  );
}

beforeEach(() => {
  mockController();
});

afterEach(() => {
  cleanup();
  bodyProps.current = undefined;
  vi.clearAllMocks();
});

describe("CoordinatorCopilot - launcher focus return", () => {
  it("returns focus to the launcher after the header Close button closes the panel", () => {
    const handleOpenChange = vi.fn();
    mockController({ handleOpenChange });
    const { rerender } = render(tree());

    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(handleOpenChange).toHaveBeenCalledWith(false);

    mockController({ handleOpenChange, open: false });
    rerender(tree());
    expect(document.activeElement).toBe(screen.getByTestId(LAUNCHER_ID));
  });

  it("does not move focus to the launcher when onClosePopover closes the panel", () => {
    const handleOpenChange = vi.fn();
    mockController({ handleOpenChange });
    const { rerender } = render(tree());
    const onClosePopover = bodyProps.current?.onClosePopover as () => void;

    onClosePopover();
    expect(handleOpenChange).toHaveBeenCalledWith(false);

    mockController({ handleOpenChange, open: false });
    rerender(tree());
    expect(document.activeElement).not.toBe(screen.getByTestId(LAUNCHER_ID));
  });

  it("hides the launcher while open and marks it busy when closed and running", () => {
    const { rerender } = render(tree());
    expect(screen.queryByTestId(LAUNCHER_ID)).toBeNull();

    mockController({
      open: false,
      launcher: { coordinator: null, loading: false, busy: true, gone: false },
    });
    rerender(tree());
    expect(screen.getByTestId(LAUNCHER_ID).getAttribute("data-busy")).toBe("true");
  });

  it("shows the busy status in the header while open", () => {
    mockController({
      launcher: { coordinator: null, loading: false, busy: true, gone: false },
    });
    render(tree());
    expect(screen.getByTestId("coordinator-copilot-busy")).toBeTruthy();
  });
});
