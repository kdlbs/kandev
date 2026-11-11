import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { CustomPrompt } from "@/lib/types/http";
import { SettingsSaveProvider } from "./settings-save-provider";
import { getPromptDraftMeta, PromptsSettings } from "./prompts-settings";

const mocks = vi.hoisted(() => ({
  createPrompt: vi.fn(),
  deletePrompt: vi.fn(),
  updatePrompt: vi.fn(),
  setPrompts: vi.fn(),
  toast: vi.fn(),
  prompts: [] as CustomPrompt[],
  finePointer: true,
  isMobile: false,
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
  useResponsiveBreakpoint: () => ({ isFinePointer: mocks.finePointer, isMobile: mocks.isMobile }),
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
        placeholder="Prompt content"
      />
    );
  },
}));

vi.mock("@/lib/api", () => ({
  createPrompt: mocks.createPrompt,
  deletePrompt: mocks.deletePrompt,
  updatePrompt: mocks.updatePrompt,
}));

const promptEditButtonTestId = "prompt-edit-button";
const saveChangesLabel = "Save changes";
const promptId = "prompt-1";
const promptName = "Review";
const promptContent = "Review this change";
const promptRowTestId = "prompt-list-item";
const promptContentInputTestId = "prompt-content-input";
const promptDeleteButtonTestId = "prompt-delete-button";
const promptDeletePopoverTestId = "prompt-delete-confirm-popover";
const promptDeleteInlineTestId = "prompt-delete-inline-confirmation";
const promptDeleteConfirmTestId = "prompt-delete-confirm";
const deleteFailureMessage = "delete failed";
const touchConfirmHeightClass = "h-11";
const touchConfirmWidthClass = "min-w-11";

beforeEach(() => {
  vi.clearAllMocks();
  mocks.prompts = [];
  mocks.finePointer = true;
  mocks.isMobile = false;
  mocks.setPrompts.mockImplementation((next: CustomPrompt[]) => {
    mocks.prompts = next;
  });
  mocks.createPrompt.mockResolvedValue({
    id: promptId,
    name: promptName,
    content: promptContent,
    builtin: false,
  });
});

afterEach(cleanup);

const existingPrompt: CustomPrompt = {
  id: promptId,
  name: promptName,
  content: promptContent,
  builtin: false,
  created_at: "2026-08-20T10:00:00Z",
  updated_at: "2026-08-20T10:00:00Z",
};

function renderPromptSettings() {
  mocks.prompts = [existingPrompt];
  render(
    <SettingsSaveProvider>
      <PromptsSettings />
    </SettingsSaveProvider>,
  );
}

describe("PromptsSettings deletion confirmation", () => {
  it("keeps the phone editor draft and delete trigger behind a confirmation sheet", async () => {
    mocks.isMobile = true;
    mocks.finePointer = false;
    renderPromptSettings();
    const row = screen.getByTestId(promptRowTestId);
    fireEvent.click(within(row).getByTestId(promptEditButtonTestId));
    const editor = within(row).getByTestId(promptContentInputTestId) as HTMLTextAreaElement;
    fireEvent.change(editor, { target: { value: "Keep the mobile draft" } });
    const trigger = within(row).getByTestId(promptDeleteButtonTestId);
    fireEvent.click(trigger);
    const sheet = screen.getByRole("dialog", { name: "Delete prompt" });
    expect(sheet.getAttribute("data-slot")).toBe("drawer-content");
    expect(trigger.isConnected).toBe(true);
    fireEvent.click(within(sheet).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(document.activeElement).toBe(trigger));
    expect(editor.value).toBe("Keep the mobile draft");
    expect(mocks.deletePrompt).not.toHaveBeenCalled();
    fireEvent.click(trigger);
    fireEvent.click(screen.getByTestId(promptDeleteConfirmTestId));
    await waitFor(() =>
      expect(mocks.deletePrompt).toHaveBeenCalledExactlyOnceWith(promptId, { cache: "no-store" }),
    );
  });
  it("cancels an anchored delete without losing the prompt editor draft", async () => {
    renderPromptSettings();

    const row = screen.getByTestId(promptRowTestId);
    fireEvent.click(within(row).getByTestId(promptEditButtonTestId));
    fireEvent.change(within(row).getByTestId(promptContentInputTestId), {
      target: { value: "Keep this draft" },
    });
    fireEvent.click(within(row).getByTestId(promptDeleteButtonTestId));

    const confirmation = screen.getByTestId(promptDeletePopoverTestId);
    expect(screen.queryByRole("alertdialog")).toBeNull();
    fireEvent.click(within(confirmation).getByRole("button", { name: "Cancel" }));

    expect(mocks.deletePrompt).not.toHaveBeenCalled();
    expect((within(row).getByTestId(promptContentInputTestId) as HTMLTextAreaElement).value).toBe(
      "Keep this draft",
    );
  });

  it("morphs the row action into touch-sized inline confirmation on coarse pointers", () => {
    mocks.finePointer = false;
    renderPromptSettings();

    const row = screen.getByTestId(promptRowTestId);
    fireEvent.click(within(row).getByTestId(promptDeleteButtonTestId));

    const inline = screen.getByTestId(promptDeleteInlineTestId);
    expect(screen.queryByTestId(promptDeletePopoverTestId)).toBeNull();
    expect(within(inline).getByTestId(promptDeleteConfirmTestId).className).toContain(
      touchConfirmHeightClass,
    );
    expect(within(inline).getByTestId(promptDeleteConfirmTestId).className).toContain(
      touchConfirmWidthClass,
    );
  });

  it("disables inline deletion while a prompt update is saving", async () => {
    mocks.finePointer = false;
    mocks.updatePrompt.mockReturnValue(new Promise<CustomPrompt>(() => undefined));
    renderPromptSettings();

    const row = screen.getByTestId(promptRowTestId);
    fireEvent.click(within(row).getByTestId(promptEditButtonTestId));
    fireEvent.change(within(row).getByTestId(promptContentInputTestId), {
      target: { value: "Save this draft" },
    });
    fireEvent.click(within(row).getByTestId(promptDeleteButtonTestId));
    fireEvent.click(screen.getByRole("button", { name: saveChangesLabel }));

    await waitFor(() => expect(mocks.updatePrompt).toHaveBeenCalledTimes(1));
    const confirm = within(screen.getByTestId(promptDeleteInlineTestId)).getByTestId(
      promptDeleteConfirmTestId,
    );
    expect(confirm.hasAttribute("disabled")).toBe(true);
    fireEvent.click(confirm);
    expect(mocks.deletePrompt).not.toHaveBeenCalled();
  });

  it("restores focus to the coarse-pointer delete button after Escape", async () => {
    mocks.finePointer = false;
    renderPromptSettings();

    const row = screen.getByTestId(promptRowTestId);
    fireEvent.click(within(row).getByTestId(promptDeleteButtonTestId));
    fireEvent.keyDown(screen.getByTestId(promptDeleteInlineTestId), { key: "Escape" });

    await waitFor(() =>
      expect(document.activeElement).toBe(within(row).getByTestId(promptDeleteButtonTestId)),
    );
  });
});

describe("PromptsSettings deletion requests", () => {
  it("closes before dispatching one delete request", async () => {
    const deleteDeferred = new Promise<void>(() => undefined);
    mocks.deletePrompt.mockReturnValue(deleteDeferred);
    renderPromptSettings();

    const row = screen.getByTestId(promptRowTestId);
    fireEvent.click(within(row).getByTestId(promptDeleteButtonTestId));
    fireEvent.click(
      within(screen.getByTestId(promptDeletePopoverTestId)).getByTestId(promptDeleteConfirmTestId),
    );

    expect(screen.queryByTestId(promptDeletePopoverTestId)).toBeNull();
    await waitFor(() => expect(mocks.deletePrompt).toHaveBeenCalledTimes(1));
    expect(mocks.deletePrompt).toHaveBeenCalledWith("prompt-1", { cache: "no-store" });
  });

  it("closes before dispatching one delete request on coarse pointers", async () => {
    mocks.finePointer = false;
    const deleteDeferred = new Promise<void>(() => undefined);
    mocks.deletePrompt.mockReturnValue(deleteDeferred);
    renderPromptSettings();

    const row = screen.getByTestId(promptRowTestId);
    fireEvent.click(within(row).getByTestId(promptDeleteButtonTestId));
    fireEvent.click(
      within(screen.getByTestId(promptDeleteInlineTestId)).getByTestId(promptDeleteConfirmTestId),
    );

    expect(screen.queryByTestId(promptDeleteInlineTestId)).toBeNull();
    await waitFor(() => expect(mocks.deletePrompt).toHaveBeenCalledTimes(1));
    expect(mocks.deletePrompt).toHaveBeenCalledWith("prompt-1", { cache: "no-store" });
  });

  it("keeps the prompt and reports localized failure feedback when deletion fails", async () => {
    mocks.deletePrompt.mockRejectedValue(new Error(deleteFailureMessage));
    renderPromptSettings();

    const row = screen.getByTestId(promptRowTestId);
    fireEvent.click(within(row).getByTestId(promptDeleteButtonTestId));
    fireEvent.click(
      within(screen.getByTestId(promptDeletePopoverTestId)).getByTestId(promptDeleteConfirmTestId),
    );

    await waitFor(() => expect(mocks.toast).toHaveBeenCalledTimes(1));
    expect(mocks.toast).toHaveBeenCalledWith({
      title: "Couldn't delete prompt",
      description: deleteFailureMessage,
      variant: "error",
    });
    expect(screen.getByTestId(promptRowTestId)).toBeTruthy();
  });

  it("keeps the prompt and reports failure feedback on coarse pointers", async () => {
    mocks.finePointer = false;
    mocks.deletePrompt.mockRejectedValue(new Error(deleteFailureMessage));
    renderPromptSettings();

    const row = screen.getByTestId(promptRowTestId);
    fireEvent.click(within(row).getByTestId(promptDeleteButtonTestId));
    fireEvent.click(
      within(screen.getByTestId(promptDeleteInlineTestId)).getByTestId(promptDeleteConfirmTestId),
    );

    await waitFor(() => expect(mocks.toast).toHaveBeenCalledTimes(1));
    expect(mocks.toast).toHaveBeenCalledWith({
      title: "Couldn't delete prompt",
      description: deleteFailureMessage,
      variant: "error",
    });
    expect(screen.getByTestId(promptRowTestId)).toBeTruthy();
  });
});

describe("PromptsSettings coordinated creation", () => {
  it("treats an opened create form as a dirty route draft", () => {
    expect(
      getPromptDraftMeta([], null, true, { name: "", content: "", allowAgentEdits: false }),
    ).toEqual({
      isDirty: true,
      revision: 'new:{"name":"","content":"","allowAgentEdits":false}',
    });
  });

  it("creates a prompt only when the floating route Save is pressed", async () => {
    render(
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Add prompt" }));

    expect(screen.getByTestId("prompt-create-form").getAttribute("data-settings-dirty")).toBe(
      "true",
    );
    expect(screen.queryByTestId("prompt-submit")).toBeNull();
    expect(screen.getByTestId("prompt-create-button").hasAttribute("disabled")).toBe(true);
    expect(mocks.createPrompt).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: saveChangesLabel }).hasAttribute("disabled")).toBe(
      true,
    );

    fireEvent.change(screen.getByPlaceholderText("Prompt name"), {
      target: { value: promptName },
    });
    fireEvent.change(screen.getByTestId(promptContentInputTestId), {
      target: { value: promptContent },
    });
    expect(mocks.promptEditor).toHaveBeenCalledWith(
      expect.objectContaining({
        promptReferences: true,
        testId: promptContentInputTestId,
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: saveChangesLabel }));

    await waitFor(() =>
      expect(mocks.createPrompt).toHaveBeenCalledWith(
        { name: promptName, content: promptContent },
        { cache: "no-store" },
      ),
    );
  });
});

describe("PromptsSettings agent permission intent", () => {
  it("preserves a remote revocation when saving an unrelated local draft", async () => {
    mocks.prompts = [{ ...existingPrompt, allow_agent_edits: true }];
    mocks.updatePrompt.mockImplementation(async (_id, patch) => ({
      ...mocks.prompts[0],
      ...patch,
    }));
    const tree = (
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>
    );
    const { rerender } = render(tree);
    fireEvent.click(screen.getByTestId(promptEditButtonTestId));
    fireEvent.change(screen.getByTestId(promptContentInputTestId), {
      target: { value: "Local draft" },
    });
    mocks.prompts = [{ ...existingPrompt, allow_agent_edits: false }];
    rerender(
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>,
    );
    expect(screen.getByRole("switch").getAttribute("aria-checked")).toBe("false");
    expect((screen.getByTestId(promptContentInputTestId) as HTMLTextAreaElement).value).toBe(
      "Local draft",
    );
    fireEvent.click(screen.getByRole("button", { name: saveChangesLabel }));
    await waitFor(() => expect(mocks.updatePrompt).toHaveBeenCalledOnce());
    expect(mocks.updatePrompt.mock.calls[0][1]).not.toHaveProperty("allow_agent_edits");
    expect(mocks.prompts[0].allow_agent_edits).toBe(false);
  });

  it("sends an explicitly changed permission with the draft", async () => {
    mocks.updatePrompt.mockResolvedValue({ ...existingPrompt, allow_agent_edits: true });
    renderPromptSettings();
    fireEvent.click(screen.getByTestId(promptEditButtonTestId));
    fireEvent.click(screen.getByRole("switch"));
    fireEvent.click(screen.getByRole("button", { name: saveChangesLabel }));
    await waitFor(() => expect(mocks.updatePrompt).toHaveBeenCalledOnce());
    expect(mocks.updatePrompt.mock.calls[0][1].allow_agent_edits).toBe(true);
  });
});

const searchLabel = "Search...";
const builtinNeedleName = "BuiltIn Needle";

describe("PromptsSettings search", () => {
  it("filters the mounted list by saved content beyond its preview", () => {
    mocks.prompts = [
      { ...existingPrompt, content: "First line\nUnique hidden body" },
      { ...existingPrompt, id: "other", name: "Other", content: "unrelated" },
    ];
    render(
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>,
    );

    fireEvent.change(screen.getByRole("searchbox", { name: searchLabel }), {
      target: { value: "hidden body" },
    });
    expect(screen.getAllByTestId(promptRowTestId)).toHaveLength(1);
    expect(screen.getByTestId(promptRowTestId).getAttribute("data-prompt-name")).toBe("Review");
  });

  it("matches literal name and raw body case insensitively, preserves order and data, and resets", () => {
    const records: CustomPrompt[] = [
      {
        ...existingPrompt,
        id: "builtin",
        name: builtinNeedleName,
        content: "first",
        builtin: true,
      },
      { ...existingPrompt, id: "body", name: "Second", content: "first\nNEEDLE [literal]" },
      { ...existingPrompt, id: "other", name: "Third", content: "different" },
    ];
    mocks.prompts = records;
    const snapshot = structuredClone(records);
    render(
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>,
    );
    const search = screen.getByRole("searchbox", { name: searchLabel });
    const names = () =>
      screen.queryAllByTestId(promptRowTestId).map((row) => row.getAttribute("data-prompt-name"));

    fireEvent.change(search, { target: { value: "  nEeDlE  " } });
    expect(names()).toEqual([builtinNeedleName, "Second"]);
    fireEvent.change(search, { target: { value: "[literal]" } });
    expect(names()).toEqual(["Second"]);
    fireEvent.change(search, { target: { value: "[missing]" } });
    expect(names()).toEqual([]);
    expect(screen.getByText("No results.")).toBeTruthy();
    expect(search.hasAttribute("disabled")).toBe(false);
    fireEvent.change(search, { target: { value: "   " } });
    expect(names()).toEqual([builtinNeedleName, "Second", "Third"]);
    fireEvent.change(search, { target: { value: "needle" } });
    fireEvent.change(search, { target: { value: "" } });
    expect(names()).toEqual([builtinNeedleName, "Second", "Third"]);
    expect(records).toEqual(snapshot);
    expect(mocks.setPrompts).not.toHaveBeenCalled();
    expect(mocks.createPrompt).not.toHaveBeenCalled();
    expect(mocks.updatePrompt).not.toHaveBeenCalled();
    expect(mocks.deletePrompt).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: saveChangesLabel })).toBeNull();
  });

  it("does not install a dirty-route guard for search, while a real draft does", async () => {
    renderPromptSettings();
    fireEvent.change(screen.getByRole("searchbox", { name: searchLabel }), {
      target: { value: "Review" },
    });
    const cleanUnload = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(cleanUnload);
    expect(cleanUnload.defaultPrevented).toBe(false);
    fireEvent.click(screen.getByTestId(promptEditButtonTestId));
    fireEvent.change(screen.getByTestId(promptContentInputTestId), {
      target: { value: "Changed" },
    });
    await waitFor(() =>
      expect(screen.getByRole("button", { name: saveChangesLabel })).toBeTruthy(),
    );
    const dirtyUnload = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(dirtyUnload);
    expect(dirtyUnload.defaultPrevented).toBe(true);
  });

  it("retains the empty collection view while searching", () => {
    render(
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>,
    );
    const empty = screen.getByText(/No prompts yet/i);
    fireEvent.change(screen.getByRole("searchbox", { name: searchLabel }), {
      target: { value: "missing" },
    });
    expect(empty.isConnected).toBe(true);
    expect(screen.queryByText("No results.")).toBeNull();
  });
});

describe("PromptsSettings search during actions", () => {
  it("disables search while editing and keeps the matched row and draft", () => {
    renderPromptSettings();
    const search = screen.getByRole("searchbox", { name: searchLabel });
    fireEvent.change(search, { target: { value: "Review" } });
    const row = screen.getByTestId(promptRowTestId);
    fireEvent.click(within(row).getByTestId(promptEditButtonTestId));
    expect(search.hasAttribute("disabled")).toBe(true);
    const editor = within(row).getByTestId(promptContentInputTestId) as HTMLTextAreaElement;
    fireEvent.change(editor, { target: { value: "Draft without the query" } });
    fireEvent.change(within(row).getByTestId("prompt-name-input"), {
      target: { value: "Renamed" },
    });
    expect(row.isConnected).toBe(true);
    expect(editor.value).toBe("Draft without the query");
    fireEvent.click(within(row).getByRole("button", { name: "Cancel" }));
    expect(search.hasAttribute("disabled")).toBe(false);
    expect(row.isConnected).toBe(true);
  });

  it("disables search while creating and during delete confirmation and request", async () => {
    mocks.deletePrompt.mockReturnValue(new Promise<void>(() => undefined));
    renderPromptSettings();
    const search = screen.getByRole("searchbox", { name: searchLabel });
    fireEvent.click(screen.getByTestId("prompt-create-button"));
    expect(search.hasAttribute("disabled")).toBe(true);
    fireEvent.click(
      within(screen.getByTestId("prompt-create-form")).getByRole("button", { name: "Cancel" }),
    );
    expect(search.hasAttribute("disabled")).toBe(false);
    fireEvent.click(screen.getByTestId(promptDeleteButtonTestId));
    expect(search.hasAttribute("disabled")).toBe(true);
    fireEvent.click(
      within(screen.getByTestId(promptDeletePopoverTestId)).getByTestId(promptDeleteConfirmTestId),
    );
    await waitFor(() => expect(mocks.deletePrompt).toHaveBeenCalledOnce());
    expect(screen.queryByTestId(promptDeletePopoverTestId)).toBeNull();
    expect(search.hasAttribute("disabled")).toBe(true);
  });
});

const noResultsText = "No results.";

describe("PromptsSettings search during saved-list refresh", () => {
  it("keeps the edited row and draft visible when a refreshed saved prompt no longer matches", () => {
    mocks.prompts = [existingPrompt];
    const tree = (
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>
    );
    const { rerender } = render(tree);
    const search = screen.getByRole("searchbox", { name: searchLabel });
    fireEvent.change(search, { target: { value: promptName } });
    const row = screen.getByTestId(promptRowTestId);
    fireEvent.click(within(row).getByTestId(promptEditButtonTestId));
    const editor = within(row).getByTestId(promptContentInputTestId) as HTMLTextAreaElement;
    fireEvent.change(editor, { target: { value: "Unsaved local draft" } });

    mocks.prompts = [{ ...existingPrompt, name: "Remote name", content: "Remote content" }];
    rerender(
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>,
    );
    expect(screen.getByTestId(promptRowTestId)).toBe(row);
    expect(within(row).getByTestId(promptContentInputTestId)).toBe(editor);
    expect(editor.value).toBe("Unsaved local draft");
    expect(search.hasAttribute("disabled")).toBe(true);

    fireEvent.click(within(row).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByTestId(promptRowTestId)).toBeNull();
    expect(screen.getByText(noResultsText)).toBeTruthy();
    expect(search.hasAttribute("disabled")).toBe(false);
  });

  it("retains refreshed edits in list order without resurrecting a removed prompt", () => {
    const rowNameAttribute = "data-prompt-name";
    const first = { ...existingPrompt, id: "first", name: "Review first" };
    const last = { ...existingPrompt, id: "last", name: "Review last" };
    mocks.prompts = [first, existingPrompt, last];
    const { rerender } = render(
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>,
    );
    fireEvent.change(screen.getByRole("searchbox", { name: searchLabel }), {
      target: { value: promptName },
    });
    fireEvent.click(
      within(screen.getAllByTestId(promptRowTestId)[1]).getByTestId(promptEditButtonTestId),
    );
    mocks.prompts = [first, { ...existingPrompt, name: "Remote", content: "Changed" }, last];
    rerender(
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>,
    );
    expect(
      screen.getAllByTestId(promptRowTestId).map((row) => row.getAttribute(rowNameAttribute)),
    ).toEqual(["Review first", "Remote", "Review last"]);
    mocks.prompts = [first, last];
    rerender(
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>,
    );
    expect(
      screen.getAllByTestId(promptRowTestId).map((row) => row.getAttribute(rowNameAttribute)),
    ).toEqual(["Review first", "Review last"]);
  });
});

describe("PromptsSettings search during delete confirmation refresh", () => {
  it("keeps the confirmation available when the saved target stops matching", () => {
    mocks.prompts = [existingPrompt];
    const { rerender } = render(
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>,
    );
    const search = screen.getByRole("searchbox", { name: searchLabel });
    fireEvent.change(search, { target: { value: promptName } });
    const row = screen.getByTestId(promptRowTestId);
    fireEvent.click(within(row).getByTestId(promptDeleteButtonTestId));
    const confirmation = screen.getByTestId(promptDeletePopoverTestId);

    mocks.prompts = [{ ...existingPrompt, name: "Remote name", content: "Remote content" }];
    rerender(
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>,
    );
    expect(screen.getByTestId(promptRowTestId)).toBe(row);
    expect(screen.getByTestId(promptDeletePopoverTestId)).toBe(confirmation);
    expect(search.hasAttribute("disabled")).toBe(true);
    fireEvent.click(within(confirmation).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByTestId(promptRowTestId)).toBeNull();
    expect(screen.getByText(noResultsText)).toBeTruthy();
    expect(search.hasAttribute("disabled")).toBe(false);
    expect(mocks.deletePrompt).not.toHaveBeenCalled();
  });
});

describe("PromptsSettings search with distinct active rows", () => {
  it("keeps both the edited row and the delete target in saved-list order", () => {
    const second = { ...existingPrompt, id: "other", name: "Review other" };
    mocks.prompts = [existingPrompt, second];
    const { rerender } = render(
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>,
    );
    const search = screen.getByRole("searchbox", { name: searchLabel });
    fireEvent.change(search, { target: { value: promptName } });
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
    rerender(
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
