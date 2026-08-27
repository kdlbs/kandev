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
  mocks.viewport.bottomOffset = 0;
  mocks.viewport.viewportBottom = 800;
});

describe("PlanBubbleMenu responsive presentation", () => {
  it("docks a focused mobile toolbar for selected text instead of mounting a selection bubble", () => {
    const editor = createEditor();

    render(<PlanBubbleMenu editor={editor} onComment={vi.fn()} mobileBottomOffset="3.25rem" />);

    expect(screen.getByRole("toolbar", { name: PLAN_TOOLBAR_LABEL })).toBeTruthy();
    expect(screen.queryByTestId("plan-selection-bubble")).toBeNull();
  });
});

describe("PlanBubbleMenu mobile selection behavior", () => {
  it("does not mount the mobile toolbar for a caret or whitespace-only selection", () => {
    const editor = createEditor({ from: 1, to: 1, text: "" });

    render(<PlanBubbleMenu editor={editor} onComment={vi.fn()} />);

    expect(screen.queryByRole("toolbar", { name: PLAN_TOOLBAR_LABEL })).toBeNull();

    act(() => {
      editor.setSelection(1, 8, "   ");
      editor.emit("transaction");
    });
    expect(screen.queryByRole("toolbar", { name: PLAN_TOOLBAR_LABEL })).toBeNull();

    act(() => {
      editor.setSelection(1, 8, "selected");
      editor.emit("transaction");
    });
    expect(screen.getByRole("toolbar", { name: PLAN_TOOLBAR_LABEL })).toBeTruthy();
  });

  it("uses compact visual actions while preserving selection on toolbar taps", () => {
    const editor = createEditor();

    render(<PlanBubbleMenu editor={editor} onComment={vi.fn()} />);

    const comment = screen.getByRole("button", { name: "editors:commentCmdShiftC" });
    expect((comment as HTMLButtonElement).disabled).toBe(false);

    const bold = screen.getByRole("button", { name: "editors:boldCmdB" });
    expect(bold.className).toContain("h-11");
    expect(bold.className).toContain("min-w-11");
    const visualSurface = bold.querySelector("span");
    expect(visualSurface?.className).toContain("h-8");
    expect(visualSurface?.className).toContain("w-8");

    const pointerDown = new Event("pointerdown", { bubbles: true, cancelable: true });
    bold.dispatchEvent(pointerDown);
    expect(pointerDown.defaultPrevented).toBe(true);

    fireEvent.click(bold);

    expect(editor.state.selection.from).toBe(1);
    expect(editor.state.selection.to).toBe(8);
    expect(editor.chainMock.focus).toHaveBeenCalledTimes(1);
    expect(editor.chainMock.toggleBold).toHaveBeenCalledTimes(1);
    expect(editor.chainMock.run).toHaveBeenCalledTimes(1);
  });
});

describe("PlanBubbleMenu responsive layout", () => {
  it("keeps the dock visible while the link input owns focus", () => {
    const editor = createEditor();

    render(<PlanBubbleMenu editor={editor} />);

    fireEvent.click(screen.getByRole("button", { name: "editors:link" }));
    expect(screen.getByRole("textbox", { name: "editors:pasteLink" })).toBeTruthy();

    act(() => {
      editor.setFocused(false);
      editor.emit("blur");
    });

    expect(screen.getByRole("toolbar", { name: PLAN_TOOLBAR_LABEL })).toBeTruthy();
  });

  it("confines a touch-tablet dock to the Plan editor bounds", () => {
    mocks.breakpoint.isMobile = false;
    mocks.breakpoint.isTablet = true;
    mocks.breakpoint.usesDesktopWorkbench = false;
    const editor = createEditor();
    const container = document.createElement("div");
    vi.spyOn(container, "getBoundingClientRect").mockReturnValue({
      left: 240,
      right: 560,
      width: 320,
      top: 0,
      bottom: 600,
      height: 600,
      x: 240,
      y: 0,
      toJSON: () => ({}),
    });
    document.body.appendChild(container);

    const renderMenu = () => (
      <PlanBubbleMenu editor={editor} mobileContainerRef={{ current: container }} />
    );
    const { rerender } = render(renderMenu());

    const toolbar = screen.getByRole("toolbar", { name: PLAN_TOOLBAR_LABEL });
    expect(toolbar.getAttribute("style")).toContain("left: 240px");
    expect(toolbar.getAttribute("style")).toContain("width: 320px");
    expect(toolbar.getAttribute("style")).toContain("right: auto");
    expect(toolbar.getAttribute("style")).toContain("top: 552px");
    expect(toolbar.getAttribute("style")).toContain("bottom: auto");

    act(() => {
      mocks.viewport.keyboardOpen = true;
      mocks.viewport.viewportBottom = 500;
      rerender(renderMenu());
    });
    expect(toolbar.getAttribute("style")).toContain("top: 452px");
    container.remove();
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

describe("PlanBubbleMenu code-block suppression", () => {
  it("hides the mobile toolbar inside a code block", () => {
    const editor = createEditor({ codeBlock: true });

    render(<PlanBubbleMenu editor={editor} onComment={vi.fn()} />);

    expect(screen.queryByRole("toolbar", { name: PLAN_TOOLBAR_LABEL })).toBeNull();
  });

  it("hides the desktop selection bubble inside a code block", () => {
    mocks.breakpoint.isFinePointer = true;
    mocks.breakpoint.isMobile = false;
    mocks.breakpoint.usesDesktopWorkbench = true;
    const editor = createEditor({ codeBlock: true });

    render(<PlanBubbleMenu editor={editor} onComment={vi.fn()} />);

    expect(screen.getByTestId("plan-selection-bubble").getAttribute("data-visible")).toBe("false");
  });
});
