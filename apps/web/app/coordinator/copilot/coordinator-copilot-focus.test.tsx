import type { ReactNode } from "react";
import { cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { OpenSequenceState } from "@/hooks/domains/coordinator/use-copilot-open-sequence";

const useCoordinatorCopilot = vi.hoisted(() => vi.fn());
const shellProps = vi.hoisted(() => ({
  current: undefined as Record<string, unknown> | undefined,
}));
const bodyProps = vi.hoisted(() => ({ current: undefined as Record<string, unknown> | undefined }));

vi.mock("./use-coordinator-copilot", () => ({ useCoordinatorCopilot }));

// Mocked so the test can invoke `onCloseAutoFocus` and `onClosePopover` directly,
// isolating the suppressCloseAutoFocusRef wiring in CoordinatorCopilot from Radix's
// actual popover lifecycle (exercised instead by e2e/tests/coordinator/proposals.spec.ts
// AC .005.6 and copilot.spec.ts AC .004.7).
vi.mock("@/components/config-chat/chat-popover-shell", () => ({
  ChatPopoverShell: (props: Record<string, unknown>) => {
    shellProps.current = props;
    return props.children as ReactNode;
  },
}));

vi.mock("./coordinator-copilot-body", () => ({
  CoordinatorCopilotBody: (props: Record<string, unknown>) => {
    bodyProps.current = props;
    return null;
  },
}));

import { CoordinatorCopilot } from "./coordinator-copilot";

const WORKSPACE_ID = "ws-1";
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

function renderCopilot() {
  return render(
    <TooltipProvider delayDuration={0}>
      <CoordinatorCopilot
        workspaceId={WORKSPACE_ID}
        coordinatorId={COORDINATOR_ID}
        coordinatorName={COORDINATOR_NAME}
        canManage
      />
    </TooltipProvider>,
  );
}

function fakeEvent() {
  return { preventDefault: vi.fn() } as unknown as Event;
}

beforeEach(() => {
  mockController();
});

afterEach(() => {
  cleanup();
  shellProps.current = undefined;
  bodyProps.current = undefined;
  vi.clearAllMocks();
});

describe("CoordinatorCopilot - close-auto-focus suppression", () => {
  it("suppresses onCloseAutoFocus once after onClosePopover, then reverts to default", () => {
    renderCopilot();
    const onClosePopover = bodyProps.current?.onClosePopover as () => void;
    const onCloseAutoFocus = shellProps.current?.onCloseAutoFocus as (event: Event) => void;

    onClosePopover();

    const firstEvent = fakeEvent();
    onCloseAutoFocus(firstEvent);
    expect(firstEvent.preventDefault).toHaveBeenCalledTimes(1);

    // Consumed once: a later close (Escape/header-Close) does not suppress.
    const secondEvent = fakeEvent();
    onCloseAutoFocus(secondEvent);
    expect(secondEvent.preventDefault).not.toHaveBeenCalled();
  });

  it("does not suppress onCloseAutoFocus for Escape/header-Close (no prior onClosePopover)", () => {
    renderCopilot();
    const onCloseAutoFocus = shellProps.current?.onCloseAutoFocus as (event: Event) => void;

    const event = fakeEvent();
    onCloseAutoFocus(event);
    expect(event.preventDefault).not.toHaveBeenCalled();
  });

  it("calls handleOpenChange(false) when onClosePopover runs", () => {
    const handleOpenChange = vi.fn();
    mockController({ handleOpenChange });
    renderCopilot();
    const onClosePopover = bodyProps.current?.onClosePopover as () => void;

    onClosePopover();

    expect(handleOpenChange).toHaveBeenCalledWith(false);
  });
});
