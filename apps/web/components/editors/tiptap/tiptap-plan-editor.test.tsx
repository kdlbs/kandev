import { act, cleanup, render, waitFor } from "@testing-library/react";
import type { Editor } from "@tiptap/core";
import { columnResizingPluginKey } from "@tiptap/pm/tables";
import { afterEach, describe, expect, it, vi } from "vitest";

const breakpoint = { isFinePointer: true, isMobile: false };
const planBubble = vi.hoisted(() => ({
  mobileBottomOffset: undefined as string | undefined,
  mobileContainerRef: undefined as { current: HTMLElement | null } | undefined,
  onMobileVisibilityChange: undefined as
    | ((visible: boolean, keyboardBottomOffset: number, keyboardOpen: boolean) => void)
    | undefined,
}));

vi.mock("@/components/theme/app-theme", () => ({
  useTheme: () => ({ resolvedTheme: "light" }),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => breakpoint,
}));

vi.mock("@/components/shared/mermaid-error-toast", () => ({
  useMermaidErrorToast: () => undefined,
}));

vi.mock("./plan-bubble-menu", () => ({
  PLAN_FORMATTING_TOOLBAR_HEIGHT_PX: 48,
  PlanBubbleMenu: (props: {
    mobileBottomOffset?: string;
    mobileContainerRef?: { current: HTMLElement | null };
    onMobileVisibilityChange?: (
      visible: boolean,
      keyboardBottomOffset: number,
      keyboardOpen: boolean,
    ) => void;
  }) => {
    planBubble.mobileBottomOffset = props.mobileBottomOffset;
    planBubble.mobileContainerRef = props.mobileContainerRef;
    planBubble.onMobileVisibilityChange = props.onMobileVisibilityChange;
    return null;
  },
}));
vi.mock("./plan-drag-handle", () => ({ PlanDragHandle: () => null }));
vi.mock("./plan-slash-menu", () => ({ PlanSlashMenu: () => null }));

import { TipTapPlanEditor } from "./tiptap-plan-editor";

const TABLE_MARKDOWN = ["| Left | Right |", "| --- | --- |", "| One | Two |"].join("\n");

function hasPluginKeyPrefix(editor: Editor, prefix: string): boolean {
  return editor.state.plugins.some((plugin) => {
    const { key } = plugin as unknown as { key?: unknown };
    return typeof key === "string" && key.startsWith(prefix);
  });
}

function getMarkdown(editor: Editor): string {
  const { markdown } = editor.storage as unknown as {
    markdown: { getMarkdown: () => string };
  };
  return markdown.getMarkdown();
}

async function renderReadyEditor(onChange: (value: string) => void): Promise<Editor> {
  let readyEditor: Editor | null = null;
  render(
    <TipTapPlanEditor
      taskId="task-1"
      value={TABLE_MARKDOWN}
      onChange={onChange}
      onEditorReady={(editor) => {
        readyEditor = editor;
      }}
    />,
  );
  await waitFor(() => expect(readyEditor).not.toBeNull());
  return readyEditor!;
}

function getFirstTableHeader(editor: Editor) {
  let headerPosition: number | null = null;
  editor.state.doc.descendants((node, position) => {
    if (node.type.name !== "tableHeader" || headerPosition !== null) return true;
    headerPosition = position;
    return false;
  });
  if (headerPosition === null) throw new Error("table header was not parsed");
  const header = editor.state.doc.nodeAt(headerPosition);
  if (!header) throw new Error("table header node was not available");
  return { header, headerPosition };
}

afterEach(() => {
  cleanup();
  breakpoint.isFinePointer = true;
  breakpoint.isMobile = false;
  planBubble.mobileBottomOffset = undefined;
  planBubble.mobileContainerRef = undefined;
  planBubble.onMobileVisibilityChange = undefined;
});

describe("TipTapPlanEditor table resizing", () => {
  it("recreates the resizable table view when pointer capability changes", async () => {
    const readyEditors: Editor[] = [];
    let latestMarkdown = TABLE_MARKDOWN;
    const handleReady = (editor: Editor) => readyEditors.push(editor);
    const handleChange = (value: string) => {
      latestMarkdown = value;
    };
    const view = render(
      <TipTapPlanEditor
        taskId="task-1"
        value={TABLE_MARKDOWN}
        onChange={handleChange}
        onEditorReady={handleReady}
      />,
    );

    await waitFor(() => {
      expect(readyEditors.length).toBe(1);
    });
    const desktopEditor = readyEditors[0];
    expect(hasPluginKeyPrefix(desktopEditor, "tableColumnResizing$")).toBe(true);
    expect(
      view.container.querySelector("table")?.parentElement?.classList.contains("tableWrapper"),
    ).toBe(true);

    act(() => {
      desktopEditor.commands.setContent(`${TABLE_MARKDOWN}\n\nDraft survives`);
    });
    await waitFor(() => expect(latestMarkdown).toContain("Draft survives"));

    breakpoint.isFinePointer = false;
    view.rerender(
      <TipTapPlanEditor
        taskId="task-1"
        value={latestMarkdown}
        onChange={handleChange}
        onEditorReady={handleReady}
      />,
    );

    await waitFor(() => {
      expect(readyEditors.length).toBe(2);
    });
    const coarsePointerEditor = readyEditors[1];
    expect(coarsePointerEditor).not.toBe(desktopEditor);
    expect(hasPluginKeyPrefix(coarsePointerEditor, "tableColumnResizing$")).toBe(false);
    expect(getMarkdown(coarsePointerEditor)).toContain("Draft survives");
    expect(
      view.container.querySelector("table")?.parentElement?.classList.contains("tableWrapper"),
    ).toBe(true);
  });

  it("keeps transient column widths out of Markdown updates", async () => {
    const onChange = vi.fn();
    const editor = await renderReadyEditor(onChange);
    const baseline = getMarkdown(editor);
    const { header, headerPosition } = getFirstTableHeader(editor);

    act(() => {
      editor.view.dispatch(
        editor.state.tr.setNodeMarkup(headerPosition, undefined, {
          ...header.attrs,
          colwidth: [160],
        }),
      );
    });

    expect(onChange).not.toHaveBeenCalled();
    expect(getMarkdown(editor)).toBe(baseline);
    expect(baseline).not.toContain("160");
  });

  it("does not emit Markdown for a native column-width transaction", async () => {
    const onChange = vi.fn();
    const editor = await renderReadyEditor(onChange);
    const { header, headerPosition } = getFirstTableHeader(editor);

    act(() => {
      editor.view.dispatch(
        editor.state.tr.setMeta(columnResizingPluginKey, {
          setDragging: { startX: 0, startWidth: 100 },
        }),
      );
    });
    onChange.mockClear();
    act(() => {
      editor.view.dispatch(
        editor.state.tr.setNodeMarkup(headerPosition, undefined, {
          ...header.attrs,
          colwidth: [160],
        }),
      );
    });

    expect(onChange).not.toHaveBeenCalled();
  });
});

describe("TipTapPlanEditor mobile formatting clearance", () => {
  it("passes the mobile navigation offset and reserves space when the dock is visible", async () => {
    breakpoint.isFinePointer = false;
    breakpoint.isMobile = true;
    const view = render(
      <TipTapPlanEditor
        taskId="task-1"
        value="Mobile plan content"
        onChange={() => undefined}
        mobileBottomOffset="3.25rem"
      />,
    );

    await waitFor(() => expect(planBubble.mobileBottomOffset).toBe("3.25rem"));
    const editorScrollContainer = view.container.querySelector(
      '[data-testid="plan-editor-scroll-container"]',
    );
    expect(editorScrollContainer).not.toBeNull();

    act(() => planBubble.onMobileVisibilityChange?.(true, 300, true));

    const editorContent = editorScrollContainer?.querySelector<HTMLElement>(".ProseMirror");
    expect(editorContent?.style.getPropertyValue("--plan-toolbar-clearance")).toBe(
      "max(48px, calc(348px - 3.25rem - env(safe-area-inset-bottom, 0px)))",
    );
    expect(planBubble.mobileContainerRef?.current).toBe(
      view.container.querySelector(".tiptap-plan-wrapper"),
    );
  });
});
