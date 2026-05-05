import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, render, fireEvent } from "@testing-library/react";
import { QuickChatTabItem } from "./quick-chat-tab-item";

afterEach(cleanup);

function makeProps(overrides: Partial<Parameters<typeof QuickChatTabItem>[0]> = {}) {
  return {
    name: "Original",
    isActive: true,
    isRenameable: true,
    onActivate: vi.fn(),
    onClose: vi.fn(),
    onRename: vi.fn(),
    ...overrides,
  };
}

const RENAME_LABEL = "Rename chat";

function startEditing(label: HTMLElement) {
  fireEvent.doubleClick(label);
}

describe("QuickChatTabItem rename", () => {
  it("commits the rename on Enter, calling onRename exactly once", () => {
    const onRename = vi.fn();
    const { getByText, getByLabelText } = render(<QuickChatTabItem {...makeProps({ onRename })} />);
    startEditing(getByText("Original"));

    const input = getByLabelText(RENAME_LABEL) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "New name" } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(onRename).toHaveBeenCalledTimes(1);
    expect(onRename).toHaveBeenCalledWith("New name");
  });

  it("discards the draft on Escape — onRename is NOT called even after blur fires on unmount", () => {
    const onRename = vi.fn();
    const { getByText, getByLabelText } = render(<QuickChatTabItem {...makeProps({ onRename })} />);
    startEditing(getByText("Original"));

    const input = getByLabelText(RENAME_LABEL) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "Should be discarded" } });
    fireEvent.keyDown(input, { key: "Escape" });

    expect(onRename).not.toHaveBeenCalled();
  });

  it("does not call onRename when the trimmed draft equals the original name", () => {
    const onRename = vi.fn();
    const { getByText, getByLabelText } = render(<QuickChatTabItem {...makeProps({ onRename })} />);
    startEditing(getByText("Original"));

    const input = getByLabelText(RENAME_LABEL) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "  Original  " } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(onRename).not.toHaveBeenCalled();
  });

  it("ignores Enter while IME composition is active", () => {
    const onRename = vi.fn();
    const { getByText, getByLabelText } = render(<QuickChatTabItem {...makeProps({ onRename })} />);
    startEditing(getByText("Original"));

    const input = getByLabelText(RENAME_LABEL) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "Composing candidate" } });
    // Enter pressed while IME is composing — should be a no-op (IME confirms candidate).
    fireEvent.keyDown(input, { key: "Enter", isComposing: true });

    expect(onRename).not.toHaveBeenCalled();
  });

  it("does not enter edit mode when isRenameable is false", () => {
    const { getByText, queryByLabelText } = render(
      <QuickChatTabItem {...makeProps({ isRenameable: false })} />,
    );
    startEditing(getByText("Original"));

    expect(queryByLabelText(RENAME_LABEL)).toBeNull();
  });
});
