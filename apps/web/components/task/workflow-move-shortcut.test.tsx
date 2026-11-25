import {
  readMoveInstructions,
  setMoveInstructions,
} from "./workflow-move-instructions-test-helpers";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { WorkflowMoveOptionsForm } from "./workflow-move-options";

vi.mock("@/hooks/domains/settings/use-custom-prompts", () => ({
  useCustomPrompts: () => ({
    prompts: [{ id: "review", name: "review-checklist", content: "Check the tests." }],
    loaded: true,
    loading: false,
  }),
}));

afterEach(cleanup);

function setup(onSubmit = vi.fn().mockResolvedValue(false), isMoving = false) {
  render(
    <TooltipProvider>
      <WorkflowMoveOptionsForm isMoving={isMoving} isTouchSurface={false} onSubmit={onSubmit} />
    </TooltipProvider>,
  );
  const input = screen.getByTestId("workflow-move-instructions");
  setMoveInstructions(input, "  Check timeout\nKeep the context  ");
  return { input, onSubmit };
}

describe("move keyboard submission", () => {
  // @covers AC-TASKS-SAVED-PROMPT-DELIVERY-001.16
  it("keeps the move shortcut available while a saved-prompt match is open", async () => {
    const { input, onSubmit } = setup();
    setMoveInstructions(input, "Check @review");
    await screen.findByRole("option", { name: "review-checklist" });

    fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledExactlyOnceWith({ instructions: "Check @review" }),
    );
  });

  // @covers AC-TASKS-KEYBOARD-ACTIONS-001.1
  it.each(["metaKey", "ctrlKey"])("submits the full draft with %s+Enter", async (modifier) => {
    const { input, onSubmit } = setup();
    fireEvent.click(screen.getByTestId("workflow-move-reset-context"));
    fireEvent.click(screen.getByTestId("workflow-move-skip-step-prompt"));
    fireEvent.keyDown(input, { key: "Enter", [modifier]: true });
    await act(async () => {});
    expect(onSubmit).toHaveBeenCalledExactlyOnceWith({
      instructions: "Check timeout\nKeep the context",
      reset_context: true,
      skip_step_prompt: true,
    });
  });

  // @covers AC-TASKS-KEYBOARD-ACTIONS-001.2
  it.each([
    {},
    { shiftKey: true },
    { metaKey: true, shiftKey: true },
    { ctrlKey: true, altKey: true },
    { metaKey: true, repeat: true },
    { metaKey: true, isComposing: true },
    { ctrlKey: true, keyCode: 229 },
  ])("leaves editing and composition alone: %j", (modifiers) => {
    const { input, onSubmit } = setup();
    fireEvent.keyDown(input, { key: "Enter", ...modifiers });
    expect(onSubmit).not.toHaveBeenCalled();
  });

  // @covers AC-TASKS-KEYBOARD-ACTIONS-001.3
  it("does not submit a disabled move", () => {
    const { input, onSubmit } = setup(vi.fn(), true);
    fireEvent.keyDown(input, { key: "Enter", metaKey: true });
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("serializes shortcut and click submissions and preserves a failed draft", async () => {
    let finish!: (value: boolean) => void;
    const onSubmit = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          finish = resolve;
        }),
    );
    const { input } = setup(onSubmit);
    act(() => {
      fireEvent.keyDown(input, { key: "Enter", metaKey: true });
      fireEvent.click(screen.getByTestId("workflow-move-submit"));
      fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    });
    expect(onSubmit).toHaveBeenCalledTimes(1);
    await act(async () => finish(false));
    expect(readMoveInstructions(input)).toBe("  Check timeout\nKeep the context  ");
    fireEvent.keyDown(input, { key: "Enter", ctrlKey: true });
    expect(onSubmit).toHaveBeenCalledTimes(2);
    await act(async () => finish(true));
    expect(readMoveInstructions(input)).toBe("");
  });
});
