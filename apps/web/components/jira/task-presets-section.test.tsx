import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { useJiraTaskPresets } from "@/components/jira/my-jira/use-task-presets";
import { DEFAULT_JIRA_PRESETS, type JiraStoredPreset } from "@/components/jira/my-jira/presets";
import { TaskPresetsSection } from "./task-presets-section";

const mocks = vi.hoisted(() => ({
  save: vi.fn(),
  reset: vi.fn(),
  toast: vi.fn(),
  promptEditor: vi.fn(),
}));

vi.mock("@/components/jira/my-jira/use-task-presets", () => ({
  useJiraTaskPresets: vi.fn(),
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));
vi.mock("@/components/settings/profile-edit/script-editor", () => ({
  ScriptEditor: () => null,
  computeEditorHeight: () => 100,
}));
vi.mock("@/components/settings/settings-prompt-editor", () => ({
  SettingsPromptEditor: (props: unknown) => {
    mocks.promptEditor(props);
    return null;
  },
}));

const customPreset: JiraStoredPreset = {
  id: "custom",
  label: "Custom",
  hint: "",
  icon: "sparkle",
  prompt_template: "",
};

describe("TaskPresetsSection", () => {
  afterEach(() => {
    cleanup();
    mocks.promptEditor.mockReset();
  });

  it("uses the shared editor with Jira action placeholders and saved prompts", () => {
    vi.mocked(useJiraTaskPresets).mockReturnValue({
      stored: [customPreset],
      isCustomized: true,
      taskPresets: [],
      save: mocks.save,
      reset: mocks.reset,
      loaded: true,
    });

    render(
      <SettingsSaveProvider>
        <TaskPresetsSection />
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Edit prompt" }));
    const props = mocks.promptEditor.mock.calls.at(-1)?.[0] as Record<string, unknown>;
    expect(props.promptReferences).toBe(true);
    expect((props.placeholders as Array<{ key: string }>).map((item) => item.key)).toEqual([
      "key",
      "url",
      "title",
      "description",
    ]);
  });

  it("stages reset-to-default until the shared save action", async () => {
    vi.mocked(useJiraTaskPresets).mockReturnValue({
      stored: [customPreset],
      isCustomized: true,
      taskPresets: [],
      save: mocks.save,
      reset: mocks.reset,
      loaded: true,
    });
    mocks.save.mockResolvedValue(undefined);

    render(
      <SettingsSaveProvider>
        <TaskPresetsSection />
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    expect(mocks.save).not.toHaveBeenCalled();
    expect(mocks.reset).not.toHaveBeenCalled();
    expect(screen.getByTestId("jira-task-presets-card").getAttribute("data-settings-dirty")).toBe(
      "true",
    );
    expect(
      screen
        .getByTestId(`jira-task-preset-${DEFAULT_JIRA_PRESETS[0].id}`)
        .getAttribute("data-settings-dirty"),
    ).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() => expect(mocks.save).toHaveBeenCalledWith(DEFAULT_JIRA_PRESETS));
    await waitFor(() =>
      expect(screen.getByTestId("jira-task-presets-card").getAttribute("data-settings-dirty")).toBe(
        "false",
      ),
    );
  });
});
