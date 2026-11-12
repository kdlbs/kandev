import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { CustomPrompt } from "@/lib/types/http";
import { SettingsSaveProvider } from "./settings-save-provider";
import { PromptsSettings } from "./prompts-settings";

const mocks = vi.hoisted(() => ({
  createPrompt: vi.fn(),
  deletePrompt: vi.fn(),
  updatePrompt: vi.fn(),
  setPrompts: vi.fn(),
  toast: vi.fn(),
  prompts: [] as CustomPrompt[],
  promptEditor: vi.fn(),
}));

vi.mock("@/hooks/domains/settings/use-custom-prompts", () => ({
  useCustomPrompts: () => ({ loaded: true }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => ({ prompts: { items: mocks.prompts } }) }),
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ prompts: { items: mocks.prompts }, setPrompts: mocks.setPrompts }),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: true, isMobile: false }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

vi.mock("@/components/settings/settings-prompt-editor", () => ({
  SettingsPromptEditor: (props: Record<string, unknown>) => {
    mocks.promptEditor(props);
    return (
      <textarea
        data-testid={props.testId as string}
        value={props.value as string}
        onChange={(event) => (props.onChange as (value: string) => void)(event.target.value)}
      />
    );
  },
}));

vi.mock("@/lib/api", () => ({
  createPrompt: mocks.createPrompt,
  deletePrompt: mocks.deletePrompt,
  updatePrompt: mocks.updatePrompt,
}));

const existingPrompt: CustomPrompt = {
  id: "prompt-1",
  name: "Review",
  content: "Review this change",
  builtin: false,
  created_at: "2026-08-20T10:00:00Z",
  updated_at: "2026-08-20T10:00:00Z",
};

const searchLabel = "Search...";
const promptRowTestId = "prompt-list-item";
const promptDeleteButtonTestId = "prompt-delete-button";
const promptDeletePopoverTestId = "prompt-delete-confirm-popover";
const promptEditButtonTestId = "prompt-edit-button";
const promptContentInputTestId = "prompt-content-input";
const remoteContent = "Remote revision";
const draftContent = "Local draft";

function renderSettings(prompts: CustomPrompt[] = [existingPrompt]) {
  mocks.prompts = prompts;
  return render(
    <SettingsSaveProvider>
      <PromptsSettings />
    </SettingsSaveProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.prompts = [];
  mocks.setPrompts.mockImplementation((next: CustomPrompt[]) => {
    mocks.prompts = next;
  });
});

afterEach(cleanup);

describe("PromptsSettings edit revisions", () => {
  it("refreshes a pristine editor from a remote prompt revision without marking it dirty", () => {
    const view = renderSettings();
    fireEvent.click(screen.getByTestId(promptEditButtonTestId));

    mocks.prompts = [
      { ...existingPrompt, content: remoteContent, updated_at: "2026-08-20T10:01:00Z" },
    ];
    view.rerender(
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>,
    );

    expect((screen.getByTestId(promptContentInputTestId) as HTMLTextAreaElement).value).toBe(
      remoteContent,
    );
    expect(screen.queryByTestId("settings-floating-save")).toBeNull();
  });

  it("preserves a dirty draft and sends the revision captured when editing began", async () => {
    mocks.updatePrompt.mockResolvedValue({
      ...existingPrompt,
      content: draftContent,
      updated_at: "2026-08-20T10:02:00Z",
    });
    const view = renderSettings();
    fireEvent.click(screen.getByTestId(promptEditButtonTestId));
    fireEvent.change(screen.getByTestId(promptContentInputTestId), {
      target: { value: draftContent },
    });

    mocks.prompts = [
      {
        ...existingPrompt,
        content: remoteContent,
        updated_at: "2026-08-20T10:01:00Z",
      },
    ];
    view.rerender(
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>,
    );
    expect((screen.getByTestId(promptContentInputTestId) as HTMLTextAreaElement).value).toBe(
      draftContent,
    );
    expect(screen.queryByRole("switch", { name: "Allow agent edits" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await waitFor(() => expect(mocks.updatePrompt).toHaveBeenCalledOnce());
    expect(mocks.updatePrompt.mock.calls[0][1]).toEqual({
      name: existingPrompt.name,
      content: draftContent,
      expected_updated_at: existingPrompt.updated_at,
    });
  });
});

describe("PromptsSettings remote list refresh", () => {
  it("keeps active rows, the dirty draft, and delete confirmation through a list update", () => {
    const second = { ...existingPrompt, id: "other", name: "Review other" };
    const view = renderSettings([existingPrompt, second]);
    const search = screen.getByRole("searchbox", { name: searchLabel });
    fireEvent.change(search, { target: { value: existingPrompt.name } });
    const [editingRow, deleteRow] = screen.getAllByTestId(promptRowTestId);
    fireEvent.click(within(editingRow).getByTestId(promptEditButtonTestId));
    fireEvent.change(within(editingRow).getByTestId(promptContentInputTestId), {
      target: { value: "Unsaved draft" },
    });
    fireEvent.click(within(deleteRow).getByTestId(promptDeleteButtonTestId));
    const confirmation = screen.getByTestId(promptDeletePopoverTestId);

    mocks.prompts = [
      { ...existingPrompt, name: "Updated first", content: "Other first" },
      { ...second, name: "Updated second", content: "Other second" },
    ];
    view.rerender(
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>,
    );

    expect(screen.getAllByTestId(promptRowTestId)).toEqual([editingRow, deleteRow]);
    expect(
      (within(editingRow).getByTestId(promptContentInputTestId) as HTMLTextAreaElement).value,
    ).toBe("Unsaved draft");
    expect(screen.getByTestId(promptDeletePopoverTestId)).toBe(confirmation);
    fireEvent.click(within(confirmation).getByRole("button", { name: "Cancel" }));
    expect(screen.getAllByTestId(promptRowTestId)).toEqual([editingRow]);
    expect(search.hasAttribute("disabled")).toBe(true);
    fireEvent.click(within(editingRow).getByRole("button", { name: "Cancel" }));
    expect(screen.queryAllByTestId(promptRowTestId)).toHaveLength(0);
    expect(search.hasAttribute("disabled")).toBe(false);
  });
});
