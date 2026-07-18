import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { SettingsSaveProvider } from "./settings-save-provider";
import { getPromptDraftMeta, PromptsSettings } from "./prompts-settings";

const mocks = vi.hoisted(() => ({
  createPrompt: vi.fn(),
  deletePrompt: vi.fn(),
  updatePrompt: vi.fn(),
  setPrompts: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/hooks/domains/settings/use-custom-prompts", () => ({
  useCustomPrompts: () => ({ loaded: true }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ prompts: { items: [] }, setPrompts: mocks.setPrompts }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

vi.mock("@/lib/api", () => ({
  createPrompt: mocks.createPrompt,
  deletePrompt: mocks.deletePrompt,
  updatePrompt: mocks.updatePrompt,
}));

beforeEach(() => {
  vi.clearAllMocks();
  mocks.createPrompt.mockResolvedValue({
    id: "prompt-1",
    name: "Review",
    content: "Review this change",
    builtin: false,
  });
});

afterEach(cleanup);

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
      target: { value: "Review" },
    });
    fireEvent.change(screen.getByPlaceholderText("Prompt content"), {
      target: { value: "Review this change" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() =>
      expect(mocks.createPrompt).toHaveBeenCalledWith(
        { name: "Review", content: "Review this change" },
        { cache: "no-store" },
      ),
    );
  });
});
