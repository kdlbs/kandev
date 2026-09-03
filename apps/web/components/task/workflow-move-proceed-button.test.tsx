import { act, cleanup, fireEvent, render, renderHook, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import {
  useWorkflowMoveLongPress,
  WORKFLOW_MOVE_LONG_PRESS_MS,
  WorkflowMoveProceedButton,
} from "./workflow-move-proceed-button";

const touchMocks = vi.hoisted(() => ({ enabled: false }));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchMocks.enabled,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      agentProfiles: { items: [] },
      availableAgents: { items: [], loading: false, loaded: true, tools: [] },
    }),
}));

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  touchMocks.enabled = false;
});

beforeEach(() => {
  vi.useFakeTimers();
});

const INSTRUCTIONS_TEST_ID = "workflow-move-instructions";
const SUBMIT_TEST_ID = "workflow-move-submit";
const PROCEED_TEST_ID = "proceed-next-step";
const OPTIONS_TEST_ID = "workflow-move-options";
const HOVER_OPTIONS_TEST_ID = `${PROCEED_TEST_ID}-options`;
const HOVER_OPEN_DELAY_MS = 120;

function renderProceed(onProceed = vi.fn()) {
  render(
    <TooltipProvider>
      <WorkflowMoveProceedButton
        nextStepName="Review"
        onProceed={onProceed}
        isMoving={false}
        testId={PROCEED_TEST_ID}
      />
    </TooltipProvider>,
  );
  return onProceed;
}

function touchPointer(overrides: Record<string, unknown> = {}) {
  return {
    pointerType: "touch",
    button: 0,
    clientX: 20,
    clientY: 20,
    ...overrides,
  } as never;
}

function openHoverForm() {
  const proceed = screen.getByTestId(PROCEED_TEST_ID);
  act(() => {
    fireEvent.mouseEnter(proceed);
    vi.advanceTimersByTime(HOVER_OPEN_DELAY_MS);
  });
  return screen.getByTestId(HOVER_OPTIONS_TEST_ID);
}

describe("WorkflowMoveProceedButton", () => {
  it("moves directly on a short desktop click", () => {
    const onProceed = renderProceed();

    fireEvent.click(screen.getByTestId(PROCEED_TEST_ID));

    expect(onProceed).toHaveBeenCalledWith(undefined);
    expect(screen.queryByTestId(HOVER_OPTIONS_TEST_ID)).toBeNull();
  });

  it("does not render a redundant options sidecar beside the direct action", () => {
    const onProceed = renderProceed();

    expect(screen.queryByTestId(`${PROCEED_TEST_ID}-options-trigger`)).toBeNull();
    expect(screen.queryByTestId(HOVER_OPTIONS_TEST_ID)).toBeNull();
    expect(onProceed).not.toHaveBeenCalled();
  });

  it("reveals the anchored options form on a desktop hover", () => {
    renderProceed();
    openHoverForm();

    expect(screen.getByTestId(HOVER_OPTIONS_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId("workflow-move-skip-step-prompt")).toBeTruthy();
    expect(screen.getByTestId(INSTRUCTIONS_TEST_ID)).toBeTruthy();
  });

  it("submits the hovered draft as one-shot entry options", async () => {
    const onProceed = vi.fn().mockResolvedValue(true);
    renderProceed(onProceed);
    openHoverForm();

    fireEvent.change(screen.getByTestId(INSTRUCTIONS_TEST_ID), {
      target: { value: "create the PR ready for review" },
    });
    fireEvent.click(screen.getByTestId(SUBMIT_TEST_ID));
    await act(async () => {});

    expect(onProceed).toHaveBeenCalledWith({
      instructions: "create the PR ready for review",
    });
    // A successful optioned move closes the hovered form.
    expect(screen.queryByTestId(HOVER_OPTIONS_TEST_ID)).toBeNull();
  });

  it("clears a successful desktop options draft before the next target", async () => {
    const onProceed = vi.fn().mockResolvedValue(true);
    const { rerender } = render(
      <TooltipProvider>
        <WorkflowMoveProceedButton
          nextStepName="Review"
          onProceed={onProceed}
          isMoving={false}
          testId={PROCEED_TEST_ID}
        />
      </TooltipProvider>,
    );
    openHoverForm();

    fireEvent.change(screen.getByTestId(INSTRUCTIONS_TEST_ID), {
      target: { value: "only for Review" },
    });
    fireEvent.click(screen.getByTestId(SUBMIT_TEST_ID));
    await act(async () => {});

    rerender(
      <TooltipProvider>
        <WorkflowMoveProceedButton
          nextStepName="QA"
          onProceed={onProceed}
          isMoving={false}
          testId={PROCEED_TEST_ID}
        />
      </TooltipProvider>,
    );
    openHoverForm();

    expect((screen.getByTestId(INSTRUCTIONS_TEST_ID) as HTMLTextAreaElement).value).toBe("");
  });

  it("keeps the hovered form and its draft open when the move fails", async () => {
    const onProceed = vi.fn().mockResolvedValue(false);
    renderProceed(onProceed);
    openHoverForm();

    fireEvent.change(screen.getByTestId(INSTRUCTIONS_TEST_ID), {
      target: { value: "retry after capacity opens" },
    });
    fireEvent.click(screen.getByTestId(SUBMIT_TEST_ID));
    await act(async () => {});

    expect(onProceed).toHaveBeenCalledOnce();
    expect(screen.getByTestId(HOVER_OPTIONS_TEST_ID)).toBeTruthy();
    expect((screen.getByTestId(INSTRUCTIONS_TEST_ID) as HTMLTextAreaElement).value).toBe(
      "retry after capacity opens",
    );
  });

  it("does not reveal the options form from a short click", () => {
    renderProceed();
    const proceed = screen.getByTestId(PROCEED_TEST_ID);

    fireEvent.pointerDown(proceed, { pointerType: "mouse", button: 0, clientX: 20, clientY: 20 });
    fireEvent.pointerUp(proceed, { pointerType: "mouse", button: 0, clientX: 20, clientY: 20 });
    fireEvent.click(proceed);

    expect(screen.queryByTestId(HOVER_OPTIONS_TEST_ID)).toBeNull();
  });

  it("closes the hovered form after the pointer leaves both the button and the form", () => {
    renderProceed();
    const proceed = screen.getByTestId(PROCEED_TEST_ID);
    openHoverForm();

    act(() => {
      fireEvent.mouseLeave(proceed);
      vi.advanceTimersByTime(200);
    });

    expect(screen.queryByTestId(HOVER_OPTIONS_TEST_ID)).toBeNull();
  });
});

describe("WorkflowMoveProceedButton — coarse pointer", () => {
  it("opens the Drawer after a long press and suppresses its duplicate click", () => {
    touchMocks.enabled = true;
    const onProceed = renderProceed();
    const proceed = screen.getByTestId(PROCEED_TEST_ID);

    fireEvent.pointerDown(proceed, touchPointer());
    act(() => vi.advanceTimersByTime(WORKFLOW_MOVE_LONG_PRESS_MS));

    expect(screen.getByTestId(OPTIONS_TEST_ID)).toBeTruthy();
    fireEvent.pointerUp(proceed, touchPointer());
    fireEvent.click(proceed);

    expect(onProceed).not.toHaveBeenCalled();
  });

  it("suppresses a long-press release retargeted to the Drawer submit action", () => {
    touchMocks.enabled = true;
    const onProceed = renderProceed();
    const proceed = screen.getByTestId(PROCEED_TEST_ID);

    fireEvent.pointerDown(proceed, touchPointer());
    act(() => vi.advanceTimersByTime(WORKFLOW_MOVE_LONG_PRESS_MS));

    fireEvent.click(screen.getByTestId(SUBMIT_TEST_ID));

    expect(onProceed).not.toHaveBeenCalled();
  });

  it("moves directly on a coarse-pointer short tap", () => {
    touchMocks.enabled = true;
    const onProceed = renderProceed();
    const proceed = screen.getByTestId(PROCEED_TEST_ID);

    fireEvent.pointerDown(proceed, touchPointer());
    fireEvent.pointerUp(proceed, touchPointer());
    fireEvent.click(proceed);

    expect(onProceed).toHaveBeenCalledWith(undefined);
    expect(screen.queryByTestId(OPTIONS_TEST_ID)).toBeNull();
  });

  it("keeps the desktop control compact and gives touch input a larger hitbox", () => {
    renderProceed();
    expect(screen.getByTestId(PROCEED_TEST_ID).className).toContain("h-6");

    cleanup();
    touchMocks.enabled = true;
    renderProceed();

    expect(screen.getByTestId(PROCEED_TEST_ID).className).toContain("min-h-11");
  });
});

describe("WorkflowMoveProceedButton — in-flight guarding", () => {
  it("suppresses rapid duplicate clicks before the move state re-renders", () => {
    const onProceed = renderProceed();
    const proceed = screen.getByTestId(PROCEED_TEST_ID);

    fireEvent.click(proceed);
    fireEvent.click(proceed);

    expect(onProceed).toHaveBeenCalledTimes(1);
  });

  it("keeps direct and options actions disabled while a move is in flight", () => {
    const onProceed = vi.fn();
    render(
      <TooltipProvider>
        <WorkflowMoveProceedButton
          nextStepName="Review"
          onProceed={onProceed}
          isMoving
          testId={PROCEED_TEST_ID}
        />
      </TooltipProvider>,
    );

    const proceed = screen.getByTestId(PROCEED_TEST_ID);
    expect((proceed as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(proceed);
    expect(onProceed).not.toHaveBeenCalled();
  });

  it("closes the hovered options form on Escape", async () => {
    renderProceed();
    const form = openHoverForm();

    fireEvent.keyDown(form, { key: "Escape" });
    act(() => vi.advanceTimersByTime(10));
    await act(async () => {});

    expect(screen.queryByTestId(HOVER_OPTIONS_TEST_ID)).toBeNull();
  });

  it("returns focus to the direct button after closing a long-press Drawer", () => {
    touchMocks.enabled = true;
    const onProceed = renderProceed();
    const proceed = screen.getByTestId(PROCEED_TEST_ID);

    fireEvent.pointerDown(proceed, touchPointer());
    act(() => vi.advanceTimersByTime(WORKFLOW_MOVE_LONG_PRESS_MS));
    fireEvent.pointerUp(proceed, touchPointer());
    fireEvent.click(proceed);
    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    act(() => vi.runOnlyPendingTimers());

    expect(onProceed).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(proceed);
  });
});

describe("useWorkflowMoveLongPress", () => {
  type PointerHandlers = ReturnType<typeof useWorkflowMoveLongPress>["pointerHandlers"];

  it.each([
    ["pointerup", (handlers: PointerHandlers) => handlers.onPointerUp(touchPointer())],
    ["pointercancel", (handlers: PointerHandlers) => handlers.onPointerCancel(touchPointer())],
  ])("cancels before the threshold on %s", (_name, cancel) => {
    const onLongPress = vi.fn();
    const { result } = renderHook(() => useWorkflowMoveLongPress(onLongPress));

    act(() => {
      result.current.pointerHandlers.onPointerDown(touchPointer());
      cancel(result.current.pointerHandlers);
      vi.advanceTimersByTime(WORKFLOW_MOVE_LONG_PRESS_MS);
    });

    expect(onLongPress).not.toHaveBeenCalled();
  });

  it("cancels when movement passes the slop without preventing scrolling", () => {
    const onLongPress = vi.fn();
    const { result } = renderHook(() => useWorkflowMoveLongPress(onLongPress));
    const moveEvent = touchPointer({ clientX: 31, clientY: 20 });

    act(() => {
      result.current.pointerHandlers.onPointerDown(touchPointer());
      result.current.pointerHandlers.onPointerMove(moveEvent);
      vi.advanceTimersByTime(WORKFLOW_MOVE_LONG_PRESS_MS);
    });

    expect(onLongPress).not.toHaveBeenCalled();
  });

  it("cancels an in-flight timer when unmounted", () => {
    const onLongPress = vi.fn();
    const { result, unmount } = renderHook(() => useWorkflowMoveLongPress(onLongPress));

    act(() => result.current.pointerHandlers.onPointerDown(touchPointer()));
    unmount();
    act(() => vi.advanceTimersByTime(WORKFLOW_MOVE_LONG_PRESS_MS));

    expect(onLongPress).not.toHaveBeenCalled();
  });

  it("marks one synthetic click as handled after a completed long press", () => {
    const onLongPress = vi.fn();
    const { result } = renderHook(() => useWorkflowMoveLongPress(onLongPress));

    act(() => {
      result.current.pointerHandlers.onPointerDown(touchPointer());
      vi.advanceTimersByTime(WORKFLOW_MOVE_LONG_PRESS_MS);
    });

    expect(onLongPress).toHaveBeenCalledOnce();
    expect(result.current.consumePendingClick()).toBe(true);
    expect(result.current.consumePendingClick()).toBe(false);
  });
});
