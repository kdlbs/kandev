import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { Editor } from "@tiptap/core";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  breakpoint: {
    isFinePointer: false,
    isMobile: true,
    isTablet: false,
    usesDesktopWorkbench: false,
  },
  viewport: {
    bottomOffset: 0,
    keyboardOpen: false,
    viewportBottom: 800,
  },
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => mocks.breakpoint,
}));

vi.mock("@/hooks/use-visual-viewport-offset", () => ({
  useVisualViewportOffset: () => mocks.viewport,
  resolveVisualViewportPosition: ({
    keyboardOpen,
    viewportBottom,
    barHeight,
    baseBottomOffset,
  }: {
    keyboardOpen: boolean;
    viewportBottom: number;
    barHeight: number;
    baseBottomOffset?: string;
  }) =>
    keyboardOpen
      ? { top: `${viewportBottom - barHeight}px`, bottom: "auto" }
      : {
          bottom: baseBottomOffset
            ? `calc(${baseBottomOffset} + env(safe-area-inset-bottom, 0px))`
            : "env(safe-area-inset-bottom, 0px)",
        },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@tiptap/react/menus", () => ({
  BubbleMenu: ({
    editor,
    shouldShow,
    children,
  }: {
    editor: Editor;
    shouldShow?: (props: { editor: Editor; state: Editor["state"] }) => boolean;
    children: React.ReactNode;
  }) => {
    const isVisible = shouldShow?.({ editor, state: editor.state }) ?? true;
    return (
      <div data-testid="plan-selection-bubble" data-visible={isVisible ? "true" : "false"}>
        {isVisible ? children : null}
      </div>
    );
  },
}));

import { PlanBubbleMenu } from "./plan-bubble-menu";

const PLAN_TOOLBAR_LABEL = "editors:planFormattingToolbar";

type FakeEditor = Editor & {
  emit: (event: string) => void;
  setSelection: (from: number, to: number, text?: string) => void;
  setFocused: (focused: boolean) => void;
  chainMock: {
    focus: ReturnType<typeof vi.fn>;
    toggleBold: ReturnType<typeof vi.fn>;
    run: ReturnType<typeof vi.fn>;
  };
};

function createEditor({
  from = 1,
  to = 8,
  text = "selected",
  focused = true,
  codeBlock = false,
}: {
  from?: number;
  to?: number;
  text?: string;
  focused?: boolean;
  codeBlock?: boolean;
} = {}): FakeEditor {
  const handlers = new Map<string, Set<() => void>>();
  const selection = { from, to };
  const chainMock = {
    focus: vi.fn(),
    toggleBold: vi.fn(),
    run: vi.fn(() => true),
  };
  chainMock.focus.mockReturnValue(chainMock);
  chainMock.toggleBold.mockReturnValue(chainMock);

  const editor = {
    isFocused: focused,
    state: {
      selection,
      doc: { textBetween: vi.fn(() => text) },
    },
    isActive: vi.fn((name: string) => name === "codeBlock" && codeBlock),
    getAttributes: vi.fn(() => ({})),
    chain: vi.fn(() => chainMock),
    commands: { focus: vi.fn() },
    view: { coordsAtPos: vi.fn(() => ({ left: 10, bottom: 20 })) },
    on: vi.fn((event: string, handler: () => void) => {
      const eventHandlers = handlers.get(event) ?? new Set<() => void>();
      eventHandlers.add(handler);
      handlers.set(event, eventHandlers);
      return editor;
    }),
    off: vi.fn((event: string, handler: () => void) => {
      handlers.get(event)?.delete(handler);
      return editor;
    }),
  } as unknown as FakeEditor;

  editor.emit = ((event: string) => {
    handlers.get(event)?.forEach((handler) => handler());
  }) as FakeEditor["emit"];
  editor.setSelection = (nextFrom, nextTo, nextText = text) => {
    selection.from = nextFrom;
    selection.to = nextTo;
    (editor.state.doc.textBetween as ReturnType<typeof vi.fn>).mockReturnValue(nextText);
  };
  editor.setFocused = (nextFocused) => {
    editor.isFocused = nextFocused;
  };
  editor.chainMock = chainMock;
  return editor;
}

afterEach(() => {
  cleanup();
  mocks.breakpoint.isFinePointer = false;
  mocks.breakpoint.isMobile = true;
  mocks.breakpoint.isTablet = false;
  mocks.breakpoint.usesDesktopWorkbench = false;
  mocks.viewport.keyboardOpen = false;
  mocks.viewport.viewportBottom = 800;
});

describe("PlanBubbleMenu responsive presentation", () => {
  it("docks a focused mobile toolbar instead of mounting a selection bubble", () => {
    const editor = createEditor();

    render(<PlanBubbleMenu editor={editor} onComment={vi.fn()} mobileBottomOffset="3.25rem" />);

    expect(screen.getByRole("toolbar", { name: PLAN_TOOLBAR_LABEL })).toBeTruthy();
    expect(screen.queryByTestId("plan-selection-bubble")).toBeNull();
  });

  it("keeps selection-only actions disabled and preserves selection on toolbar taps", () => {
    const editor = createEditor({ from: 1, to: 1, text: "" });

    render(<PlanBubbleMenu editor={editor} onComment={vi.fn()} />);

    const comment = screen.getByRole("button", { name: "editors:commentCmdShiftC" });
    expect((comment as HTMLButtonElement).disabled).toBe(true);

    const bold = screen.getByRole("button", { name: "editors:boldCmdB" });
    const pointerDown = new Event("pointerdown", { bubbles: true, cancelable: true });
    bold.dispatchEvent(pointerDown);
    expect(pointerDown.defaultPrevented).toBe(true);

    fireEvent.click(bold);

    expect(editor.state.selection.from).toBe(1);
    expect(editor.state.selection.to).toBe(1);
    expect(editor.chainMock.focus).toHaveBeenCalledTimes(1);
    expect(editor.chainMock.toggleBold).toHaveBeenCalledTimes(1);
    expect(editor.chainMock.run).toHaveBeenCalledTimes(1);
  });

  it("preserves the desktop selection bubble for fine-pointer layouts", () => {
    mocks.breakpoint.isFinePointer = true;
    mocks.breakpoint.isMobile = false;
    mocks.breakpoint.usesDesktopWorkbench = true;
    const editor = createEditor();

    render(<PlanBubbleMenu editor={editor} onComment={vi.fn()} />);

    expect(screen.getByTestId("plan-selection-bubble").getAttribute("data-visible")).toBe("true");
    expect(screen.queryByRole("toolbar", { name: PLAN_TOOLBAR_LABEL })).toBeNull();
  });

  it("updates mobile visibility from editor focus and transaction events", () => {
    const editor = createEditor({ focused: false });
    render(<PlanBubbleMenu editor={editor} />);

    expect(screen.queryByRole("toolbar", { name: PLAN_TOOLBAR_LABEL })).toBeNull();

    act(() => {
      editor.setFocused(true);
      editor.emit("focus");
    });
    expect(screen.getByRole("toolbar", { name: PLAN_TOOLBAR_LABEL })).toBeTruthy();

    act(() => {
      editor.setFocused(false);
      editor.emit("blur");
    });
    expect(screen.queryByRole("toolbar", { name: PLAN_TOOLBAR_LABEL })).toBeNull();
  });
});
