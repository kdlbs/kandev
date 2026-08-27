import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { WorkflowStep } from "@/lib/types/http";
import { StepPromptSection } from "./workflow-step-prompt-section";

const promptEditor = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/domains/settings/use-custom-prompts", () => ({
  useCustomPrompts: () => ({ prompts: [], loaded: true, loading: false }),
}));
vi.mock("./settings-prompt-editor", () => ({
  SettingsPromptEditor: (props: Record<string, unknown>) => {
    promptEditor(props);
    return <div data-testid={props.testId as string} />;
  },
}));

const step = {
  id: "step-1",
  workflow_id: "workflow-1",
  name: "Implement",
  prompt: "Implement {{task_prompt}}",
} as WorkflowStep;

afterEach(() => {
  cleanup();
  promptEditor.mockReset();
});

describe("StepPromptSection", () => {
  it("uses shared prompt references and step placeholders", () => {
    render(
      <StepPromptSection
        step={step}
        savedStep={{ ...step, prompt: "Previous prompt" }}
        localPrompt={step.prompt ?? ""}
        onLocalPromptChange={vi.fn()}
        debouncedUpdatePrompt={vi.fn()}
        readOnly={false}
      />,
    );

    expect(promptEditor).toHaveBeenCalledWith(
      expect.objectContaining({
        promptReferences: true,
        isDirty: true,
        placeholders: expect.arrayContaining([expect.objectContaining({ key: "task_prompt" })]),
      }),
    );
  });
});
