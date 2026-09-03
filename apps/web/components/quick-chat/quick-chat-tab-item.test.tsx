import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, render, fireEvent } from "@testing-library/react";
import { QuickChatTabItem, type QuickChatTabDragProps } from "./quick-chat-tab-item";

const responsiveMock = vi.hoisted(() => ({ isFinePointer: true }));

vi.mock("@kandev/ui/context-menu", () => ({
  ContextMenu: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  ContextMenuTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  ContextMenuContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  ContextMenuItem: ({
    children,
    onSelect,
  }: {
    children: React.ReactNode;
    onSelect?: () => void;
  }) => (
    <button type="button" onClick={onSelect}>
      {children}
    </button>
  ),
}));

vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DropdownMenuItem: ({
    children,
    onSelect,
    ...props
  }: {
    children: React.ReactNode;
    onSelect?: () => void;
    [key: string]: unknown;
  }) => (
    <button type="button" {...props} onClick={() => onSelect?.()}>
      {children}
    </button>
  ),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsiveMock,
}));

afterEach(() => {
  cleanup();
  responsiveMock.isFinePointer = true;
});

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
  it("starts renaming from the tab context menu", () => {
    const { getByRole, getByLabelText } = render(<QuickChatTabItem {...makeProps()} />);

    fireEvent.contextMenu(getByRole("button", { name: "Original" }));
    fireEvent.click(getByRole("button", { name: "Rename" }));

    expect(getByLabelText(RENAME_LABEL)).toBeTruthy();
  });

  it("renders an accessible configuration indicator", () => {
    const { getByLabelText } = render(
      <QuickChatTabItem
        {...makeProps()}
        {...({ kind: "config" } as Partial<Parameters<typeof QuickChatTabItem>[0]>)}
      />,
    );

    expect(getByLabelText("Configuration chat")).toBeTruthy();
  });

  it("uses the tab surface as the sortable activator without a visible drag handle", () => {
    const onActivate = vi.fn();
    const onDragStart = vi.fn();
    const setActivatorNodeRef = vi.fn();
    const dragProps = {
      attributes: { "aria-roledescription": "sortable" },
      listeners: { onPointerDown: onDragStart },
      setActivatorNodeRef,
    } as unknown as QuickChatTabDragProps;

    const { getByRole, getByTestId, queryByTestId } = render(
      <QuickChatTabItem {...makeProps({ onActivate })} dragProps={dragProps} />,
    );

    const tabSurface = getByTestId("quick-chat-tab");
    const tabButton = getByRole("button", { name: /^Original$/ });
    expect(tabSurface.getAttribute("aria-roledescription")).toBe("sortable");
    expect(queryByTestId("quick-chat-tab-drag-handle")).toBeNull();
    expect(setActivatorNodeRef).toHaveBeenCalledWith(tabSurface);

    fireEvent.click(tabButton);
    expect(onActivate).toHaveBeenCalledOnce();
    fireEvent.pointerDown(tabSurface);
    expect(onDragStart).toHaveBeenCalledOnce();
  });
});

describe("QuickChatTabItem coarse-pointer actions", () => {
  it("exposes reachable rename, reorder, and close actions with touch targets", () => {
    responsiveMock.isFinePointer = false;
    const onMoveLeft = vi.fn();
    const onMoveRight = vi.fn();
    const onClose = vi.fn();

    const { getByRole, getAllByRole, getByLabelText } = render(
      <QuickChatTabItem
        {...makeProps({ onMoveLeft, onMoveRight, onClose })}
        canMoveLeft
        canMoveRight
      />,
    );

    const actions = getByRole("button", { name: "Actions for Original" });
    expect(actions.className).toContain("h-11");
    expect(actions.className).toContain("w-11");

    fireEvent.click(actions);
    fireEvent.click(getByRole("button", { name: "Move Original left" }));
    fireEvent.click(getByRole("button", { name: "Move Original right" }));
    fireEvent.click(getByRole("button", { name: "Close Original" }));

    expect(onMoveLeft).toHaveBeenCalledOnce();
    expect(onMoveRight).toHaveBeenCalledOnce();
    expect(onClose).toHaveBeenCalledOnce();

    fireEvent.click(getAllByRole("button", { name: "Rename" })[0]);
    expect(getByLabelText(RENAME_LABEL)).toBeTruthy();
  });
});

describe("QuickChatTabItem rename actions", () => {
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

  it("shows edit mode clearly without Save or Cancel buttons", () => {
    const onRename = vi.fn();
    const { getByText, getByLabelText, getByTestId, queryByRole, queryByLabelText } = render(
      <QuickChatTabItem {...makeProps({ onRename })} />,
    );
    startEditing(getByText("Original"));

    const input = getByLabelText(RENAME_LABEL) as HTMLInputElement;
    expect(input.className).toContain("border-primary");
    expect(input.className).toContain("bg-accent");
    expect(input.className).toContain("text-base");
    const tabSurface = getByTestId("quick-chat-tab");
    expect(tabSurface.className).toContain("border-primary");
    expect(tabSurface.className).toContain("bg-accent");
    expect(queryByRole("button", { name: "Save" })).toBeNull();
    expect(queryByRole("button", { name: "Cancel" })).toBeNull();

    fireEvent.change(input, { target: { value: "  Saved name  " } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(onRename).toHaveBeenCalledTimes(1);
    expect(onRename).toHaveBeenCalledWith("Saved name");
    expect(queryByLabelText(RENAME_LABEL)).toBeNull();
  });

  it("restores the previous name with Escape without calling onRename", () => {
    const onRename = vi.fn();
    const { getByText, getByLabelText, queryByLabelText } = render(
      <QuickChatTabItem {...makeProps({ onRename })} />,
    );
    startEditing(getByText("Original"));

    const input = getByLabelText(RENAME_LABEL);
    fireEvent.change(input, { target: { value: "Draft" } });
    fireEvent.keyDown(input, { key: "Escape" });

    expect(onRename).not.toHaveBeenCalled();
    expect(queryByLabelText(RENAME_LABEL)).toBeNull();
    expect(getByText("Original")).toBeTruthy();
  });

  it("commits once when editing ends through blur", () => {
    const onRename = vi.fn();
    const { getByText, getByLabelText } = render(<QuickChatTabItem {...makeProps({ onRename })} />);
    startEditing(getByText("Original"));

    const input = getByLabelText(RENAME_LABEL) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "Saved once" } });
    fireEvent.blur(input);

    expect(onRename).toHaveBeenCalledTimes(1);
    expect(onRename).toHaveBeenCalledWith("Saved once");
  });
});

describe("QuickChatTabItem rename edge cases", () => {
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

describe("QuickChatTabItem activity", () => {
  it("shows the grid spinner while its conversation is working", () => {
    const { getByRole } = render(<QuickChatTabItem {...makeProps({ isWorking: true })} />);

    const spinner = getByRole("status", { name: "Loading" });
    expect(spinner).toBeTruthy();
    expect(spinner.closest("button")?.className).toContain("gap-1.5");
  });

  it("does not show a spinner for a settled conversation", () => {
    const { queryByRole } = render(<QuickChatTabItem {...makeProps({ isWorking: false })} />);

    expect(queryByRole("status", { name: "Loading" })).toBeNull();
  });
});
