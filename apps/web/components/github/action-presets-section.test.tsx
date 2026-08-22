import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";

const mocks = vi.hoisted(() => ({
  usePresets: vi.fn(),
  promptEditor: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/hooks/domains/github/use-github-action-presets", () => ({
  useGitHubActionPresets: mocks.usePresets,
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));
vi.mock("@/components/settings/profile-edit/script-editor", () => ({
  ScriptEditor: () => null,
  computeEditorHeight: () => "80px",
}));
vi.mock("@/components/settings/settings-prompt-editor", () => ({
  SettingsPromptEditor: (props: unknown) => {
    mocks.promptEditor(props);
    return null;
  },
}));

import { ActionPresetsSection } from "./action-presets-section";

describe("ActionPresetsSection prompt editor", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("uses the shared editor with GitHub action placeholders and saved prompts", () => {
    mocks.usePresets.mockReturnValue({
      presets: {
        pr: [
          {
            id: "review",
            label: "Review",
            hint: "Review it",
            icon: "eye",
            prompt_template: "Review {{title}}",
          },
        ],
        issue: [],
      },
      save: vi.fn(),
      reset: vi.fn(),
      loading: false,
    });

    render(
      <SettingsSaveProvider>
        <ActionPresetsSection workspaceId="workspace-1" />
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Edit prompt" }));
    const props = mocks.promptEditor.mock.calls.at(-1)?.[0] as Record<string, unknown>;
    expect(props.promptReferences).toBe(true);
    expect((props.placeholders as Array<{ key: string }>).map((item) => item.key)).toEqual([
      "url",
      "title",
    ]);
    expect(props.isDirty).toBe(false);
  });
});
