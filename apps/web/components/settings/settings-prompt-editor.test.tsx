import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  promptState: {
    prompts: [
      { id: "review", name: "review-helper", content: "Review the change." },
      { id: "release", name: "release-notes", content: "Write release notes." },
    ],
    loaded: true,
    loading: false,
  },
  scriptEditor: vi.fn(),
  useCustomPrompts: vi.fn(),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock("@/hooks/domains/settings/use-custom-prompts", () => ({
  useCustomPrompts: (options: unknown) => {
    mocks.useCustomPrompts(options);
    return mocks.promptState;
  },
}));

vi.mock("./profile-edit/script-editor", () => ({
  ScriptEditor: (props: unknown) => {
    mocks.scriptEditor(props);
    return null;
  },
  computeEditorHeight: (value: string, minLines: number) => `${value.length + minLines}px`,
}));

import { SettingsPromptEditor } from "./settings-prompt-editor";

describe("SettingsPromptEditor", () => {
  afterEach(() => {
    cleanup();
    mocks.scriptEditor.mockReset();
    mocks.useCustomPrompts.mockReset();
    mocks.promptState.loaded = true;
    mocks.promptState.loading = false;
  });

  it("passes placeholder and saved-prompt completion modes to its controlled editor", () => {
    const placeholders = [
      { key: "title", description: "Title", example: "Bug", executor_types: [] },
    ];

    const { container } = render(
      <SettingsPromptEditor
        value="Review @release"
        onChange={() => undefined}
        placeholders={placeholders}
        promptReferences
        excludedPromptIds={["review"]}
        minLines={5}
        isDirty
        ariaLabel="Prompt editor"
        testId="prompt-editor"
      />,
    );

    const props = mocks.scriptEditor.mock.calls.at(-1)?.[0] as Record<string, unknown>;
    expect(props.placeholders).toBe(placeholders);
    expect(props.mentionPrompts).toEqual([mocks.promptState.prompts[1]]);
    expect(props.height).toBe("20px");
    expect(props.ariaLabel).toBe("Prompt editor");
    expect(props.testId).toBe("prompt-editor-editor");
    expect(document.body.textContent).toContain("settings:promptEditorBothHint");
    expect(container.firstElementChild?.getAttribute("data-settings-dirty")).toBe("true");
    expect(container.firstElementChild?.getAttribute("data-settings-dirty-level")).toBe(
      "container",
    );
  });

  it("does not offer saved prompts while the prompt list is loading", () => {
    mocks.promptState.loading = true;
    mocks.promptState.loaded = false;

    render(
      <SettingsPromptEditor
        value="{{title}}"
        onChange={() => undefined}
        placeholders={[]}
        promptReferences
      />,
    );

    const props = mocks.scriptEditor.mock.calls.at(-1)?.[0] as Record<string, unknown>;
    expect(props.mentionPrompts).toEqual([]);
  });

  it("keeps utility-style editors on placeholder completion only", () => {
    render(
      <SettingsPromptEditor
        value="{{workspace}}"
        onChange={() => undefined}
        placeholders={[
          { key: "workspace", description: "Workspace", example: "repo", executor_types: [] },
        ]}
      />,
    );

    const props = mocks.scriptEditor.mock.calls.at(-1)?.[0] as Record<string, unknown>;
    expect(props.mentionPrompts).toBeUndefined();
    expect(document.body.textContent).toContain("settings:promptEditorPlaceholderHint");
    expect(mocks.useCustomPrompts).toHaveBeenLastCalledWith({ enabled: false });
  });

  it("shows saved-prompt guidance when placeholders are not available", () => {
    render(<SettingsPromptEditor value="Review @" onChange={() => undefined} promptReferences />);

    expect(document.body.textContent).toContain("settings:promptEditorPromptReferenceHint");
  });
});
