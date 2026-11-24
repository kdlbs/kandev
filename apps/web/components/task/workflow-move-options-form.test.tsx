import {
  readMoveInstructions,
  setMoveInstructions,
} from "./workflow-move-instructions-test-helpers";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { WorkflowMoveOptions } from "./workflow-move-options";

const touchMocks = vi.hoisted(() => ({ enabled: false }));

const RESET_TEST_ID = "workflow-move-reset-context";
const SKIP_TEST_ID = "workflow-move-skip-step-prompt";
const INSTRUCTIONS_TEST_ID = "workflow-move-instructions";
const SUBMIT_TEST_ID = "workflow-move-submit";

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => touchMocks.enabled,
}));

vi.mock("@/hooks/domains/settings/use-custom-prompts", () => ({
  useCustomPrompts: () => ({ prompts: [], loaded: true, loading: false }),
}));

afterEach(() => {
  cleanup();
  touchMocks.enabled = false;
});

function renderOptions(onSubmit = vi.fn(), onOpenChange = vi.fn(), touch = true) {
  touchMocks.enabled = touch;
  render(
    <TooltipProvider>
      <WorkflowMoveOptions
        open
        onOpenChange={onOpenChange}
        targetStepName="Review"
        onSubmit={onSubmit}
      />
    </TooltipProvider>,
  );
  return { onSubmit, onOpenChange };
}

describe("WorkflowMoveOptions", () => {
  it("exposes the reset-context, skip-step-prompt, and instruction overrides", () => {
    renderOptions();

    expect(screen.getByTestId(RESET_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(SKIP_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(INSTRUCTIONS_TEST_ID)).toBeTruthy();
  });

  // @covers AC-TASKS-SAVED-PROMPT-DELIVERY-001.15
  it("exposes an accessible saved-prompt editor in the drawer", () => {
    renderOptions();

    const editor = screen.getByRole("textbox", { name: "One-time instructions" });
    expect(editor.getAttribute("contenteditable")).toBe("true");
    expect(editor.getAttribute("aria-autocomplete")).toBe("list");
  });

  it("submits the draft payload and keeps values when the move fails", async () => {
    const onSubmit = vi.fn().mockResolvedValue(false);
    renderOptions(onSubmit);

    setMoveInstructions(
      screen.getByTestId(INSTRUCTIONS_TEST_ID),
      "  create the PR ready for review  ",
    );
    fireEvent.click(screen.getByTestId(SUBMIT_TEST_ID));
    await act(async () => {});

    expect(onSubmit).toHaveBeenCalledWith({
      instructions: "create the PR ready for review",
    });
    expect(readMoveInstructions(screen.getByTestId(INSTRUCTIONS_TEST_ID))).toBe(
      "  create the PR ready for review  ",
    );
  });

  it("includes the skip-step-prompt flag in the one-time payload", async () => {
    const onSubmit = vi.fn().mockResolvedValue(true);
    renderOptions(onSubmit);

    fireEvent.click(screen.getByTestId(SKIP_TEST_ID));
    fireEvent.click(screen.getByTestId(SUBMIT_TEST_ID));
    await act(async () => {});

    expect(onSubmit).toHaveBeenCalledWith({ skip_step_prompt: true });
  });
});

describe("WorkflowMoveOptions presentation", () => {
  it("closes through the cancel action", () => {
    const { onOpenChange } = renderOptions();

    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));

    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("uses the Drawer presentation on touch surfaces", () => {
    touchMocks.enabled = true;
    renderOptions();

    expect(screen.getByTestId("workflow-move-options")).toBeTruthy();
    expect(document.querySelector('[data-slot="drawer-content"]')).toBeTruthy();
  });

  it("uses the Dialog presentation on fine-pointer surfaces", () => {
    renderOptions(vi.fn(), vi.fn(), false);

    expect(screen.queryByTestId("workflow-move-options")).toBeNull();
    expect(document.querySelector('[data-slot="dialog-content"]')).toBeNull();
    expect(document.querySelector('[data-slot="drawer-content"]')).toBeNull();
  });
});
