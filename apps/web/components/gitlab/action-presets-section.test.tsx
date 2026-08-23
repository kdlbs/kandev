import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";

const mocks = vi.hoisted(() => ({
  usePresets: vi.fn(),
  promptEditor: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/hooks/domains/gitlab/use-gitlab-action-presets", () => ({
  useGitLabActionPresets: mocks.usePresets,
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));
vi.mock("@/components/settings/settings-prompt-editor", () => ({
  SettingsPromptEditor: (props: unknown) => {
    mocks.promptEditor(props);
    return null;
  },
}));

import { GitLabActionPresetsSection } from "./action-presets-section";

describe("GitLabActionPresetsSection prompt editor", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("uses the shared editor with GitLab action placeholders and saved prompts", () => {
    mocks.usePresets.mockReturnValue({
      presets: {
        mr: [
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
      loading: false,
      update: vi.fn(),
      reset: vi.fn(),
    });

    render(
      <SettingsSaveProvider>
        <GitLabActionPresetsSection workspaceId="workspace-1" />
      </SettingsSaveProvider>,
    );

    const props = mocks.promptEditor.mock.calls.at(-1)?.[0] as Record<string, unknown>;
    expect(props.promptReferences).toBe(true);
    expect((props.placeholders as Array<{ key: string }>).map((item) => item.key)).toEqual([
      "url",
      "title",
    ]);
  });
});
