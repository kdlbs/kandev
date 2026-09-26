import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { Workflow } from "@/lib/types/http";
import { WorkflowPromptSection } from "./workflow-prompt-section";

const PROMPT_INPUT = "workflow-prompt-input";
const promptEditor = vi.hoisted(() => vi.fn());

vi.mock("./settings-prompt-editor", () => ({
  SettingsPromptEditor: (props: Record<string, unknown>) => {
    promptEditor(props);
    return (
      <textarea
        data-testid={props.testId as string}
        data-settings-dirty={String(props.isDirty)}
        value={props.value as string}
        onChange={(event) => (props.onChange as (value: string) => void)(event.target.value)}
        readOnly={props.readOnly as boolean}
      />
    );
  },
}));

const baseWorkflow = {
  id: "workflow-1",
  workspace_id: "workspace-1",
  name: "Workflow",
  description: "",
  created_at: "",
  updated_at: "",
} as Workflow;

afterEach(() => {
  cleanup();
  promptEditor.mockReset();
});

describe("WorkflowPromptSection", () => {
  it("starts collapsed when the prompt is empty", () => {
    render(<WorkflowPromptSection workflow={baseWorkflow} onUpdate={vi.fn()} />);

    expect(screen.getByTestId("workflow-prompt-toggle")).toBeTruthy();
    expect(screen.queryByTestId(PROMPT_INPUT)).toBeNull();
    expect(screen.getByText(/Optional/i)).toBeTruthy();
  });

  it("starts expanded when a non-empty prompt is loaded", () => {
    const workflow = {
      ...baseWorkflow,
      prompt: "If the PR is merged or closed, move the Task to Done.",
    };
    render(<WorkflowPromptSection workflow={workflow} onUpdate={vi.fn()} />);

    const input = screen.getByTestId(PROMPT_INPUT) as HTMLTextAreaElement;
    expect(input.value).toBe(workflow.prompt);
    expect(promptEditor).toHaveBeenCalledWith(
      expect.objectContaining({
        promptReferences: true,
        testId: PROMPT_INPUT,
        isDirty: true,
      }),
    );
  });

  it("auto-expands when an empty prompt becomes non-empty", () => {
    const onUpdate = vi.fn();
    const { rerender } = render(
      <WorkflowPromptSection workflow={baseWorkflow} onUpdate={onUpdate} />,
    );
    expect(screen.queryByTestId(PROMPT_INPUT)).toBeNull();

    rerender(
      <WorkflowPromptSection
        workflow={{ ...baseWorkflow, prompt: "Keep multi-PR work in subtasks." }}
        onUpdate={onUpdate}
      />,
    );
    expect(screen.getByTestId(PROMPT_INPUT)).toBeTruthy();
  });

  it("toggles open via the header control", () => {
    render(<WorkflowPromptSection workflow={baseWorkflow} onUpdate={vi.fn()} />);
    fireEvent.click(screen.getByTestId("workflow-prompt-toggle"));
    expect(screen.getByTestId(PROMPT_INPUT)).toBeTruthy();
  });

  it("marks the textarea dirty when the draft prompt differs from saved", () => {
    const draft = { ...baseWorkflow, prompt: "new" };
    const saved = { ...baseWorkflow, prompt: "old" };
    render(<WorkflowPromptSection workflow={draft} savedWorkflow={saved} onUpdate={vi.fn()} />);
    const input = screen.getByTestId(PROMPT_INPUT);
    expect(input.getAttribute("data-settings-dirty")).toBe("true");
  });
});
