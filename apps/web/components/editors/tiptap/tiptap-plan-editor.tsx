"use client";

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useTheme } from "next-themes";
import { useEditor, EditorContent } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import Placeholder from "@tiptap/extension-placeholder";
import Link from "@tiptap/extension-link";
import Highlight from "@tiptap/extension-highlight";
import Underline from "@tiptap/extension-underline";
import TaskList from "@tiptap/extension-task-list";
import TaskItem from "@tiptap/extension-task-item";
import { Table } from "@tiptap/extension-table";
import { TableRow } from "@tiptap/extension-table-row";
import { TableCell } from "@tiptap/extension-table-cell";
import { TableHeader } from "@tiptap/extension-table-header";
import { Markdown } from "tiptap-markdown";
import { createCodeBlockWithMermaid } from "./tiptap-mermaid-extension";
import { common, createLowlight } from "lowlight";
import {
  CommentMark,
  rehydrateCommentMarks,
  MIN_COMMENT_TEXT_LENGTH,
  type CommentForEditor,
} from "./comment-mark";
import { createPlanSlashExtension, type PlanSlashCommand } from "./plan-slash-commands";
import { PlanSlashMenu } from "./plan-slash-menu";
import { PlanBubbleMenu } from "./plan-bubble-menu";
import { PlanDragHandle } from "./plan-drag-handle";
import type { MenuState } from "@/components/task/chat/tiptap-suggestion";
import { DOMParser as PmDOMParser } from "@tiptap/pm/model";
import type { Editor } from "@tiptap/core";

export type { CommentForEditor };

export type TextSelection = {
  text: string;
  from?: number;
  to?: number;
  position: { x: number; y: number };
};

type TipTapPlanEditorProps = {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  onSelectionChange?: (selection: TextSelection | null) => void;
  comments?: CommentForEditor[];
  onCommentClick?: (id: string, position: { x: number; y: number }) => void;
  onCommentDeleted?: (ids: string[]) => void;
  onEditorReady?: (editor: Editor) => void;
};

const lowlight = createLowlight(common);

/** Regex matching common markdown syntax signals. */
const MD_SIGNALS = /^#{1,6}\s|^\s*[-*+]\s|^\s*\d+\.\s|```|\*\*|__|\[.+\]\(/m;

/**
 * Creates a paste handler that detects when pasted HTML is just a `<pre>`
 * wrapper around text (e.g. copying raw markdown from GitHub) and re-parses
 * the clipboard text/plain through the tiptap-markdown parser instead.
 */
function createPasteHandler(editorRef: React.RefObject<Editor | null>) {
  return (_view: import("@tiptap/pm/view").EditorView, event: ClipboardEvent): boolean => {
    const html = event.clipboardData?.getData("text/html");
    const text = event.clipboardData?.getData("text/plain");
    if (!html || !text) return false;

    // Only intercept when the HTML is a bare <pre> wrapper (no rich content)
    const trimmed = html.replace(/^<meta[^>]*>/, "").trim();
    if (!trimmed.match(/^<pre[^>]*>[\s\S]*<\/pre>$/i)) return false;

    // Must look like markdown
    if (!MD_SIGNALS.test(text)) return false;

    const ed = editorRef.current;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const parser = (ed?.storage as any)?.markdown?.parser;
    if (!parser) return false;

    event.preventDefault();
    const parsed: string = parser.parse(text);
    const div = document.createElement("div");
    div.innerHTML = parsed;
    const slice = PmDOMParser.fromSchema(ed!.state.schema).parseSlice(div, {
      preserveWhitespace: true,
    });
    const { from, to } = ed!.state.selection;
    ed!.view.dispatch(ed!.state.tr.replaceRange(from, to, slice));
    return true;
  };
}

/** Build the TipTap editor extensions array. */
function buildEditorExtensions(
  placeholder: string,
  slashExtension: ReturnType<typeof createPlanSlashExtension>,
  onOrphanedComments: (ids: string[]) => void,
) {
  return [
    StarterKit.configure({ codeBlock: false }),
    createCodeBlockWithMermaid(lowlight),
    Markdown.configure({ html: true, transformPastedText: true, transformCopiedText: true }),
    Placeholder.configure({ placeholder }),
    Link.configure({ openOnClick: false }),
    Highlight,
    Underline,
    TaskList,
    TaskItem.configure({ nested: true }),
    Table.configure({ resizable: false }),
    TableRow,
    TableCell,
    TableHeader,
    CommentMark.configure({ onOrphanedComments }),
    slashExtension,
  ];
}

/** Hook for Cmd+Shift+C keyboard shortcut to trigger comment popover. */
function useCommentShortcut(
  wrapperRef: React.RefObject<HTMLDivElement | null>,
  editorRef: React.RefObject<ReturnType<typeof useEditor> | null>,
  onSelectionChangeRef: React.RefObject<((sel: TextSelection | null) => void) | undefined>,
) {
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (!((e.metaKey || e.ctrlKey) && e.shiftKey && e.key === "c")) return;
      if (!onSelectionChangeRef.current) return;

      e.preventDefault();
      e.stopImmediatePropagation();

      const ed = editorRef.current;
      if (!ed) return;

      const { from, to } = ed.state.selection;
      if (from === to) return;
      const text = ed.state.doc.textBetween(from, to, " ").trim();
      if (text.length < MIN_COMMENT_TEXT_LENGTH) return;

      const endCoords = ed.view.coordsAtPos(to);
      onSelectionChangeRef.current({
        text,
        from,
        to,
        position: { x: endCoords.left, y: endCoords.bottom },
      });
    };

    wrapper.addEventListener("keydown", handleKeyDown, true);
    return () => wrapper.removeEventListener("keydown", handleKeyDown, true);
  }, [wrapperRef, editorRef, onSelectionChangeRef]);
}

const EMPTY_SLASH_STATE: MenuState<PlanSlashCommand> = {
  isOpen: false,
  items: [],
  query: "",
  clientRect: null,
  command: null,
};

/** Handle keyboard events for slash menu navigation. */
function handleSlashKeyDown(
  event: KeyboardEvent,
  menu: MenuState<PlanSlashCommand>,
  setIndex: React.Dispatch<React.SetStateAction<number>>,
  indexRef: React.RefObject<number>,
): boolean {
  if (!menu.isOpen || !menu.items.length) return false;
  const len = menu.items.length;
  if (event.key === "ArrowDown") {
    setIndex((p) => (p + 1) % len);
    return true;
  }
  if (event.key === "ArrowUp") {
    setIndex((p) => (p - 1 + len) % len);
    return true;
  }
  if (event.key === "Enter") {
    const item = menu.items[indexRef.current];
    if (item && menu.command) menu.command(item);
    return true;
  }
  return false;
}

/** Slash command menu state and keyboard navigation. */
function useSlashMenu() {
  const [menuState, setMenuState] = useState<MenuState<PlanSlashCommand>>(EMPTY_SLASH_STATE);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const selectedIndexRef = useRef(0);
  useEffect(() => {
    setSelectedIndex(0);
  }, [menuState.items]);

  // Current handler — recreated when menuState changes, captures latest state
  const keyDown = useCallback(
    (event: KeyboardEvent) =>
      handleSlashKeyDown(event, menuState, setSelectedIndex, selectedIndexRef),
    [menuState],
  );
  // Ref synced via layout effect so it's always current
  const keyDownRef = useRef<(event: KeyboardEvent) => boolean>(keyDown);
  useLayoutEffect(() => {
    keyDownRef.current = keyDown;
  });
  useLayoutEffect(() => {
    selectedIndexRef.current = selectedIndex;
  });

  // Stable callback passed to extension — never changes, delegates via ref
  const stableKeyDown = useCallback((event: KeyboardEvent) => keyDownRef.current(event), []);

  /* eslint-disable react-hooks/refs -- stableKeyDown reads ref for deferred access, not during render */
  const extension = useMemo(
    () => createPlanSlashExtension(setMenuState, stableKeyDown),
    [stableKeyDown],
  );
  /* eslint-enable react-hooks/refs */

  return { menuState, selectedIndex, setSelectedIndex, extension };
}

/** Comment highlight / badge click handler via event delegation. */
function useCommentClickHandler(
  wrapperRef: React.RefObject<HTMLDivElement | null>,
  onCommentClickRef: React.RefObject<
    ((id: string, pos: { x: number; y: number }) => void) | undefined
  >,
) {
  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper) return;
    const handleClick = (e: MouseEvent) => {
      if (!onCommentClickRef.current) return;
      const target = e.target as HTMLElement;
      // Check badge first, then the mark wrapper (comment-highlight)
      const badge = target.closest(".comment-badge");
      if (badge) {
        const commentId = badge.getAttribute("data-comment-id");
        if (commentId) {
          e.preventDefault();
          e.stopPropagation();
          onCommentClickRef.current(commentId, { x: e.clientX, y: e.clientY });
        }
        return;
      }
      // Clicked on highlighted text — read commentId directly from the mark span
      const highlight = target.closest(".comment-highlight");
      if (!highlight) return;
      const commentId = highlight.getAttribute("data-comment-id");
      if (commentId) {
        e.preventDefault();
        e.stopPropagation();
        onCommentClickRef.current(commentId, { x: e.clientX, y: e.clientY });
      }
    };
    wrapper.addEventListener("click", handleClick);
    return () => wrapper.removeEventListener("click", handleClick);
  }, [wrapperRef, onCommentClickRef]);
}

type PlanEditorState = {
  editor: Editor | null;
  editorRef: React.RefObject<Editor | null>;
  onSelectionChangeRef: React.RefObject<((sel: TextSelection | null) => void) | undefined>;
  onCommentClickRef: React.RefObject<
    ((id: string, pos: { x: number; y: number }) => void) | undefined
  >;
  isReady: boolean;
  slash: ReturnType<typeof useSlashMenu>;
};

/** Hook encapsulating TipTap editor setup, extensions, and lifecycle effects. */
function usePlanEditor(props: TipTapPlanEditorProps): PlanEditorState {
  const {
    value,
    onChange,
    placeholder = "Start typing...",
    onSelectionChange,
    comments = [],
    onCommentClick,
    onCommentDeleted,
    onEditorReady,
  } = props;

  const editorRef = useRef<ReturnType<typeof useEditor>>(null);
  const onChangeRef = useRef(onChange);
  const onSelectionChangeRef = useRef(onSelectionChange);
  const onCommentClickRef = useRef(onCommentClick);
  const onCommentDeletedRef = useRef(onCommentDeleted);
  const onEditorReadyRef = useRef(onEditorReady);
  const [isReady, setIsReady] = useState(false);

  const slash = useSlashMenu();
  /* eslint-disable react-hooks/refs -- createPasteHandler reads ref for deferred access in event handler, not during render */
  const pasteHandler = useMemo(() => createPasteHandler(editorRef), []);
  /* eslint-enable react-hooks/refs */

  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);
  useEffect(() => {
    onSelectionChangeRef.current = onSelectionChange;
  }, [onSelectionChange]);
  useEffect(() => {
    onCommentClickRef.current = onCommentClick;
  }, [onCommentClick]);
  useEffect(() => {
    onCommentDeletedRef.current = onCommentDeleted;
  }, [onCommentDeleted]);
  useEffect(() => {
    onEditorReadyRef.current = onEditorReady;
  }, [onEditorReady]);

  const stableOrphanHandler = useCallback((ids: string[]) => {
    onCommentDeletedRef.current?.(ids);
  }, []);

  /* eslint-disable react-hooks/refs -- stableOrphanHandler reads ref for deferred access, not during render */
  const extensions = useMemo(
    () => buildEditorExtensions(placeholder, slash.extension, stableOrphanHandler),
    [placeholder, slash.extension, stableOrphanHandler],
  );
  /* eslint-enable react-hooks/refs */

  const editor = useEditor({
    immediatelyRender: false,
    extensions,
    content: value,
    editorProps: {
      attributes: { class: "tiptap-plan-editor", spellcheck: "false" },
      handlePaste: pasteHandler,
    },
    onUpdate: ({ editor: ed }) => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const md = (ed.storage as any).markdown?.getMarkdown?.() as string | undefined;
      onChangeRef.current(md ?? ed.getText());
    },
    onCreate: ({ editor: ed }) => {
      setIsReady(true);
      onEditorReadyRef.current?.(ed);
    },
  });

  useEffect(() => {
    editorRef.current = editor;
  }, [editor]);

  useEffect(() => {
    if (!editor || !isReady) return;
    try {
      rehydrateCommentMarks(editor, comments);
    } catch {
      /* editor may be transitional */
    }
  }, [comments, editor, isReady]);

  return { editor, editorRef, onSelectionChangeRef, onCommentClickRef, isReady, slash };
}

export function TipTapPlanEditor(props: TipTapPlanEditorProps) {
  const { resolvedTheme } = useTheme();
  const wrapperRef = useRef<HTMLDivElement>(null);
  const { editor, editorRef, onSelectionChangeRef, onCommentClickRef, isReady, slash } =
    usePlanEditor(props);

  useCommentClickHandler(wrapperRef, onCommentClickRef);
  useCommentShortcut(wrapperRef, editorRef, onSelectionChangeRef);

  const handleBubbleComment = useCallback(
    (sel: TextSelection) => {
      onSelectionChangeRef.current?.(sel);
    },
    [onSelectionChangeRef],
  );

  return (
    <div
      ref={wrapperRef}
      className={`tiptap-plan-wrapper markdown-body h-full relative ${resolvedTheme === "dark" ? "dark" : ""}`}
    >
      <EditorContent editor={editor} className="h-full" />
      {editor && isReady && (
        <>
          <PlanBubbleMenu editor={editor} onComment={handleBubbleComment} />
          <PlanDragHandle editor={editor} />
        </>
      )}
      <PlanSlashMenu
        menuState={slash.menuState}
        selectedIndex={slash.selectedIndex}
        setSelectedIndex={slash.setSelectedIndex}
      />
      {!isReady && (
        <div className="absolute inset-0 flex items-center justify-center text-muted-foreground text-sm bg-background/80">
          Loading editor...
        </div>
      )}
    </div>
  );
}
