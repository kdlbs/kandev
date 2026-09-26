import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PromptSection } from "./prompt-section";

const promptEditor = vi.hoisted(() => vi.fn());

vi.mock("@/components/settings/settings-prompt-editor", () => ({
  SettingsPromptEditor: (props: Record<string, unknown>) => {
    promptEditor(props);
    return <div data-testid="automation-prompt-editor" />;
  },
}));

afterEach(() => {
  cleanup();
  promptEditor.mockReset();
});

describe("PromptSection", () => {
  it("uses shared prompt references and automation placeholders", () => {
    render(
      <PromptSection
        value="Run {{pr.title}}"
        isDirty
        onChange={vi.fn()}
        placeholders={[{ key: "pr.title", description: "PR title", example: "Ship it" }]}
      />,
    );

    expect(promptEditor).toHaveBeenCalledWith(
      expect.objectContaining({
        promptReferences: true,
        language: "plaintext",
        isDirty: true,
        placeholders: expect.arrayContaining([expect.objectContaining({ key: "pr.title" })]),
      }),
    );
  });
});
