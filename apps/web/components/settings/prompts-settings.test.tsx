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
  promptEditor: vi.fn(),
}));

vi.mock("@/hooks/domains/settings/use-custom-prompts", () => ({
  useCustomPrompts: () => ({ loaded: true }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ prompts: { items: mocks.prompts }, setPrompts: mocks.setPrompts }),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: mocks.finePointer }),
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
  it("cancels an anchored delete without losing the prompt editor draft", async () => {
    renderPromptSettings();

    const row = screen.getByTestId(promptRowTestId);
    fireEvent.click(within(row).getByTestId("prompt-edit-button"));
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
    fireEvent.click(within(row).getByTestId("prompt-edit-button"));
    fireEvent.change(within(row).getByTestId(promptContentInputTestId), {
      target: { value: "Save this draft" },
    });
    fireEvent.click(within(row).getByTestId(promptDeleteButtonTestId));
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

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
    expect(getPromptDraftMeta([], null, true, { name: "", content: "" })).toEqual({
      isDirty: true,
      revision: 'new:{"name":"","content":""}',
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
    expect(screen.getByRole("button", { name: "Save changes" }).hasAttribute("disabled")).toBe(
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
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(mocks.createPrompt).toHaveBeenCalledWith(
        { name: promptName, content: promptContent },
        { cache: "no-store" },
      ),
    );
  });
});
