import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";

import type { CustomPrompt } from "@/lib/types/http";
import { SettingsSaveProvider } from "./settings-save-provider";
import { PromptsSettings } from "./prompts-settings";

const mocks = vi.hoisted(() => ({
  prompts: [] as CustomPrompt[],
  promptEditor: vi.fn(),
  saveVersions: vi.fn(),
}));

vi.mock("@/lib/api/domains/prompt-sync-api", () => ({
  savePromptVersions: (...args: unknown[]) => mocks.saveVersions(...args),
}));
vi.mock("@/components/settings/prompt-sync-section", () => ({ PromptSyncSection: () => null }));
vi.mock("@/hooks/domains/settings/use-custom-prompts", () => ({
  useCustomPrompts: () => ({ loaded: true }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => ({ prompts: { items: mocks.prompts } }) }),
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ prompts: { items: mocks.prompts }, setPrompts: vi.fn() }),
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: true, isMobile: false }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("@/components/settings/settings-prompt-editor", () => ({
  SettingsPromptEditor: (props: Record<string, unknown>) => {
    mocks.promptEditor(props);
    return (
      <textarea
        data-testid={props.testId as string}
        value={props.value as string}
        readOnly={props.readOnly as boolean}
        onChange={(event) => (props.onChange as (value: string) => void)(event.target.value)}
      />
    );
  },
}));
vi.mock("@/lib/api", () => ({
  createPrompt: vi.fn(),
  deletePrompt: vi.fn(),
  updatePrompt: vi.fn(),
}));

const rowTestId = "prompt-list-item";
const saveChangesLabel = "Save changes";
const previewTestId = "prompt-version-preview";
const CHECKED = "aria-checked";

const base: CustomPrompt = {
  id: "s1",
  name: "hook",
  content: "Repository text",
  alternate_content: "My text",
  builtin: false,
  source: "sync",
  source_path: "p/hook.md",
  created_at: "2026-08-20T10:00:00Z",
  updated_at: "2026-08-20T10:00:00Z",
};
const override: CustomPrompt = {
  ...base,
  id: "o1",
  name: "other",
  content: "My other text",
  alternate_content: "Repository other text",
  source: "override",
  source_path: "p/other.md",
};

beforeEach(() => {
  vi.clearAllMocks();
  mocks.prompts = [];
  mocks.saveVersions.mockImplementation(async (id: string) =>
    mocks.prompts.find((prompt) => prompt.id === id),
  );
});

afterEach(cleanup);

function renderPrompts() {
  render(
    <TooltipProvider>
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>
    </TooltipProvider>,
  );
}

function row(name: string) {
  return screen
    .getAllByTestId(rowTestId)
    .find((item) => item.getAttribute("data-prompt-name") === name)!;
}

function versionSwitch(name: string) {
  return screen.getByRole("switch", { name: `Use the repository version of @${name}` });
}

// @covers AC-INTEGRATIONS-PROMPT-SYNC-007.1 AC-INTEGRATIONS-PROMPT-SYNC-007.2 AC-INTEGRATIONS-PROMPT-SYNC-009.2
describe("pending version switches", () => {
  it("uses the review step's Local | Repository control, on for the repository", () => {
    mocks.prompts = [base, override];
    renderPrompts();
    expect(versionSwitch("hook").getAttribute(CHECKED)).toBe("true");
    expect(versionSwitch("other").getAttribute(CHECKED)).toBe("false");
    expect(within(row("hook")).getByText("Local")).toBeTruthy();
    expect(within(row("hook")).getByText("Repository")).toBeTruthy();
    expect(within(row("hook")).queryByText("Local override")).toBeNull();
  });

  it("previews the version a switched prompt will use, applying nothing yet", () => {
    mocks.prompts = [base];
    renderPrompts();
    fireEvent.click(versionSwitch("hook"));
    expect(versionSwitch("hook").getAttribute(CHECKED)).toBe("false");
    const preview = within(row("hook")).getByTestId(previewTestId) as HTMLTextAreaElement;
    expect(preview.value).toBe("My text");
    expect(preview.readOnly).toBe(true);
    expect(within(row("hook")).getByTestId("prompt-edit-button")).toBeTruthy();
    expect(screen.getByRole("button", { name: saveChangesLabel })).toBeTruthy();
    expect(mocks.saveVersions).not.toHaveBeenCalled();
  });

  it("saves several switched prompts with one Save", async () => {
    mocks.prompts = [base, override];
    renderPrompts();
    fireEvent.click(versionSwitch("hook"));
    fireEvent.click(versionSwitch("other"));
    const preview = within(row("other")).getByTestId(previewTestId) as HTMLTextAreaElement;
    expect(preview.value).toBe("Repository other text");
    fireEvent.click(screen.getByRole("button", { name: saveChangesLabel }));
    await waitFor(() => expect(mocks.saveVersions).toHaveBeenCalledTimes(2));
    expect(mocks.saveVersions.mock.calls.map((call) => [call[0], call[1]])).toEqual([
      ["s1", { use_local: true }],
      ["o1", { use_local: false }],
    ]);
  });

  it("saves an edit to a switched prompt's local version with its switch", async () => {
    mocks.prompts = [base];
    renderPrompts();
    fireEvent.click(versionSwitch("hook"));
    fireEvent.click(within(row("hook")).getByTestId("prompt-edit-button"));
    const editor = within(row("hook")).getByTestId("prompt-content-input") as HTMLTextAreaElement;
    expect(editor.value).toBe("My text");
    expect(editor.readOnly).toBe(false);
    expect(
      (within(row("hook")).getByTestId("prompt-name-input") as HTMLInputElement).disabled,
    ).toBe(true);
    fireEvent.change(editor, { target: { value: "Edited" } });
    fireEvent.click(screen.getByRole("button", { name: saveChangesLabel }));
    await waitFor(() => expect(mocks.saveVersions).toHaveBeenCalledOnce());
    expect(mocks.saveVersions.mock.calls[0]![1]).toEqual({
      use_local: true,
      local_content: "Edited",
      expected_updated_at: base.updated_at,
    });
  });

  it("restores every pending switch on Reset", () => {
    mocks.prompts = [base, override];
    renderPrompts();
    fireEvent.click(versionSwitch("hook"));
    fireEvent.click(versionSwitch("other"));
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    expect(versionSwitch("hook").getAttribute(CHECKED)).toBe("true");
    expect(versionSwitch("other").getAttribute(CHECKED)).toBe("false");
    expect(screen.queryByTestId(previewTestId)).toBeNull();
    expect(mocks.saveVersions).not.toHaveBeenCalled();
  });
});
