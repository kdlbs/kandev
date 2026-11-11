import { cleanup, createEvent, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TabRenameInput } from "./tab-rename-input";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// @covers AC-UI-EDIT-FOCUS-001.4
it.each([
  { key: "Enter", isComposing: true, keyCode: 13 },
  { key: "Enter", isComposing: false, keyCode: 229 },
  { key: "Escape", isComposing: true, keyCode: 27 },
  { key: "Escape", isComposing: false, keyCode: 229 },
])("leaves $key ($isComposing, $keyCode) to the IME without resolving rename", (key) => {
  const commit = vi.fn();
  const cancel = vi.fn();
  const surroundingKey = vi.fn();
  render(
    <div onKeyDown={surroundingKey}>
      <TabRenameInput initial="Original" seqBadge={null} onCommit={commit} onCancel={cancel} />
    </div>,
  );
  const input = screen.getByRole("textbox");
  fireEvent.compositionStart(input);
  fireEvent.change(input, { target: { value: "日本" } });
  const event = createEvent.keyDown(input, { ...key, bubbles: true, cancelable: true });
  fireEvent(input, event);
  expect(event.defaultPrevented).toBe(false);
  expect(surroundingKey).not.toHaveBeenCalled();
  expect(commit).not.toHaveBeenCalled();
  expect(cancel).not.toHaveBeenCalled();
  expect((input as HTMLInputElement).value).toBe("日本");

  fireEvent.compositionEnd(input, { data: "日本語" });
  fireEvent.change(input, { target: { value: "日本語" } });
  fireEvent.keyDown(input, { key: "Enter", isComposing: false, keyCode: 13 });
  fireEvent.keyDown(input, { key: "Enter" });
  fireEvent.blur(input, { relatedTarget: document.body });
  expect(commit).toHaveBeenCalledExactlyOnceWith("日本語");
  expect(cancel).not.toHaveBeenCalled();
});

// @covers AC-UI-EDIT-FOCUS-001.1
it("does not commit a tab rename when the document loses focus", () => {
  vi.spyOn(document, "hasFocus").mockReturnValue(false);
  const commit = vi.fn();
  render(<TabRenameInput initial="Original" seqBadge={1} onCommit={commit} onCancel={vi.fn()} />);
  const input = screen.getByRole("textbox") as HTMLInputElement;
  fireEvent.change(input, { target: { value: "Unfinished name" } });
  fireEvent.blur(input);
  expect(commit).not.toHaveBeenCalled();
  expect(input.value).toBe("Unfinished name");
  fireEvent.keyDown(input, { key: "Enter" });
  fireEvent.keyDown(input, { key: "Enter" });
  fireEvent.blur(input, { relatedTarget: document.body });
  expect(commit).toHaveBeenCalledExactlyOnceWith("Unfinished name");
});

it("cancels on Escape without a subsequent blur commit", () => {
  const commit = vi.fn();
  const cancel = vi.fn();
  render(<TabRenameInput initial="Original" seqBadge={null} onCommit={commit} onCancel={cancel} />);
  const input = screen.getByRole("textbox");
  fireEvent.keyDown(input, { key: "Escape" });
  fireEvent.blur(input, { relatedTarget: document.body });
  expect(commit).not.toHaveBeenCalled();
  expect(cancel).toHaveBeenCalledTimes(1);
});

it("retains the native UTF-16 limit for ordinary tab inputs", () => {
  render(
    <TabRenameInput
      initial="Terminal"
      seqBadge={null}
      maxLength={120}
      onCommit={vi.fn()}
      onCancel={vi.fn()}
    />,
  );
  expect((screen.getByRole("textbox") as HTMLInputElement).maxLength).toBe(120);
});
