"use client";

import { useRef, useImperativeHandle, useEffect, useLayoutEffect, useMemo } from "react";
import { useEditor, ReactNodeViewRenderer } from "@tiptap/react";
import Document from "@tiptap/extension-document";
import Paragraph from "@tiptap/extension-paragraph";
import Text from "@tiptap/extension-text";
import HardBreak from "@tiptap/extension-hard-break";
import History from "@tiptap/extension-history";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { matchesShortcut } from "@/lib/keyboard/utils";
import { getShortcut, type StoredShortcutOverrides } from "@/lib/keyboard/shortcut-overrides";
import { Node as ProseMirrorNode } from "@tiptap/pm/model";
import { Decoration, DecorationSet } from "@tiptap/pm/view";
import { Extension, isNodeEmpty } from "@tiptap/core";
import { useHistoryKeymap } from "./tiptap-editor-history";
import Code from "@tiptap/extension-code";
import CodeBlockLowlight from "@tiptap/extension-code-block-lowlight";
import { common, createLowlight } from "lowlight";
import { cn } from "@/lib/utils";
import { useAppStore } from "@/components/state-provider";
import { getChatDraftContent, setChatDraftContent } from "@/lib/local-storage";
import { getMarkdownText, textToHtml, handleEditorPaste } from "./tiptap-helpers";
import { CodeBlockView } from "./tiptap-code-block-view";
import { ContextMention } from "./tiptap-mention-extension";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { TaskMentionData } from "@/hooks/use-inline-mention";

export type TipTapInputHandle = {
  focus: () => void;
  blur: () => void;
  getSelectionStart: () => number;
  getValue: () => string;
  setValue: (value: string) => void;
  clear: () => void;
  getTextareaElement: () => HTMLElement | null;
  insertText: (text: string, from: number, to: number) => void;
  getMentions: () => ContextFile[];
  getTaskMentions: () => TaskMentionData[];
};

const lowlightInstance = createLowlight(common);

/**
 * Custom Placeholder extension that reads the current placeholder from
 * editor.storage instead of from the captured options object. TipTap's
 * extension.options is a getter returning a fresh object each call, so the
 * built-in Placeholder plugin's closure captures a stale options snapshot.
 * This wrapper uses editor.storage.dynamicPlaceholder.text as the live source.
 */
const DynamicPlaceholder = Extension.create({
  name: "dynamicPlaceholder",

  addStorage() {
    return { text: "" };
  },

  addProseMirrorPlugins() {
    const editor = this.editor;
    return [
      new Plugin({
        key: new PluginKey("dynamicPlaceholder"),
        props: {
          decorations: ({ doc, selection }) => {
            if (!editor.isEditable && !editor.isEmpty) return null;
            const { anchor } = selection;
            const decorations: InstanceType<typeof Decoration>[] = [];
            const isEmptyDoc = editor.isEmpty;
            doc.descendants((node: ProseMirrorNode, pos: number) => {
              const hasAnchor = anchor >= pos && anchor <= pos + node.nodeSize;
              const isEmpty = !node.isLeaf && isNodeEmpty(node);
              if (hasAnchor && isEmpty) {
                const classes = ["is-empty"];
                if (isEmptyDoc) classes.push("is-editor-empty");
                decorations.push(
                  Decoration.node(pos, pos + node.nodeSize, {
                    class: classes.join(" "),
                    // eslint-disable-next-line @typescript-eslint/no-explicit-any
                    "data-placeholder": (editor.storage as any).dynamicPlaceholder.text,
                  }),
                );
              }
              return false;
            });
            return DecorationSet.create(doc, decorations);
          },
        },
      }),
    ];
  },
});

/** Update the dynamic placeholder text and trigger a redecoration. Extracted to a
 *  standalone function so that the eslint react-hooks/immutability rule does not
 *  flag it as a mutation of the `editor` hook argument. */
function updateDynamicPlaceholder(ed: ReturnType<typeof useEditor>, text: string) {
  if (!ed) return;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (ed.storage as any).dynamicPlaceholder.text = text;
  ed.view.dispatch(ed.state.tr);
}

type UseTipTapEditorOptions = {
  value: string;
  onChange: (value: string) => void;
  onSubmit?: () => void;
  placeholder: string;
  disabled: boolean;
  className?: string;
  planModeEnabled: boolean;
  onPlanModeChange?: (enabled: boolean) => void;
  submitKey: "enter" | "cmd_enter";
  onFocus?: () => void;
  onBlur?: () => void;
  sessionId: string | null;
  onImagePaste?: (files: File[]) => void;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  mentionSuggestion: any;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  slashSuggestion: any;
  /** True when a slash/@ suggestion menu is open with selectable items. Enter
   *  must defer to the suggestion plugin so the highlighted item is inserted
   *  instead of submitting the message. */
  isSuggestionMenuOpen: boolean;
  /** Returns the user's previous messages for this session, newest-first. The
   *  caller maintains the actual list; the editor reads it on each keypress so
   *  ArrowUp/ArrowDown navigate the latest history without prop churn. */
  getHistory: () => readonly string[];
  /** Open the Ctrl+R fuzzy search overlay. The overlay lives in the parent
   *  component (so it can position itself relative to the editor) — the editor
   *  only knows when to open it. */
  onOpenReverseSearch: () => void;
  /** True while the reverse-search overlay owns focus. The editor must ignore
   *  ArrowUp/ArrowDown in that state so the overlay's own list navigation
   *  isn't shadowed. */
  isReverseSearchOpen: boolean;
  ref: React.ForwardedRef<TipTapInputHandle>;
};

function useTipTapRefs(opts: UseTipTapEditorOptions) {
  const onSubmitRef = useRef(opts.onSubmit);
  const submitKeyRef = useRef(opts.submitKey);
  const disabledRef = useRef(opts.disabled);
  const onChangeRef = useRef(opts.onChange);
  const onImagePasteRef = useRef(opts.onImagePaste);
  const sessionIdRef = useRef(opts.sessionId);
  const planModeEnabledRef = useRef(opts.planModeEnabled);
  const onPlanModeChangeRef = useRef(opts.onPlanModeChange);
  const isSuggestionMenuOpenRef = useRef(opts.isSuggestionMenuOpen);
  const getHistoryRef = useRef(opts.getHistory);
  const onOpenReverseSearchRef = useRef(opts.onOpenReverseSearch);
  const isReverseSearchOpenRef = useRef(opts.isReverseSearchOpen);
  const keyboardShortcuts = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const keyboardShortcutsRef = useRef(keyboardShortcuts);
  useLayoutEffect(() => {
    onSubmitRef.current = opts.onSubmit;
    submitKeyRef.current = opts.submitKey;
    disabledRef.current = opts.disabled;
    onChangeRef.current = opts.onChange;
    onImagePasteRef.current = opts.onImagePaste;
    sessionIdRef.current = opts.sessionId;
    planModeEnabledRef.current = opts.planModeEnabled;
    onPlanModeChangeRef.current = opts.onPlanModeChange;
    isSuggestionMenuOpenRef.current = opts.isSuggestionMenuOpen;
    getHistoryRef.current = opts.getHistory;
    onOpenReverseSearchRef.current = opts.onOpenReverseSearch;
    isReverseSearchOpenRef.current = opts.isReverseSearchOpen;
    keyboardShortcutsRef.current = keyboardShortcuts;
  });
  return {
    onSubmitRef,
    submitKeyRef,
    disabledRef,
    onChangeRef,
    onImagePasteRef,
    sessionIdRef,
    planModeEnabledRef,
    onPlanModeChangeRef,
    isSuggestionMenuOpenRef,
    getHistoryRef,
    onOpenReverseSearchRef,
    isReverseSearchOpenRef,
    keyboardShortcutsRef,
  };
}

function buildEditorExtensions(args: {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  mentionSuggestion: any;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  slashSuggestion: any;
  submitKeymap: Extension;
  historyKeymap: Extension;
}) {
  return [
    Document,
    Paragraph,
    Text,
    HardBreak,
    History,
    Code,
    CodeBlockLowlight.extend({
      addNodeView() {
        return ReactNodeViewRenderer(CodeBlockView);
      },
    }).configure({ lowlight: lowlightInstance }),
    DynamicPlaceholder,
    ContextMention.configure({
      suggestions: [args.mentionSuggestion, args.slashSuggestion],
    }),
    args.submitKeymap,
    args.historyKeymap,
  ];
}

function buildEditorProps(args: {
  planModeEnabled: boolean;
  className: string | undefined;
  onFocus: (() => void) | undefined;
  onBlur: (() => void) | undefined;
  onImagePasteRef: React.RefObject<((files: File[]) => void) | undefined>;
}) {
  return {
    attributes: {
      class: cn(
        "w-full h-full resize-none bg-transparent px-2 py-2 overflow-y-auto",
        "text-sm leading-relaxed",
        "placeholder:text-muted-foreground",
        "focus:outline-none",
        "disabled:cursor-not-allowed disabled:opacity-50",
        args.planModeEnabled && "border-primary/40",
        args.className,
      ),
    },
    handlePaste: (view: import("@tiptap/pm/view").EditorView, event: ClipboardEvent) =>
      handleEditorPaste(view, event, args.onImagePasteRef),
    handleDOMEvents: {
      focus: () => {
        args.onFocus?.();
        return false;
      },
      blur: () => {
        args.onBlur?.();
        return false;
      },
    },
  };
}

export function useTipTapEditor(opts: UseTipTapEditorOptions) {
  const refs = useTipTapRefs(opts);
  const SubmitKeymap = useSubmitKeymap({
    disabledRef: refs.disabledRef,
    submitKeyRef: refs.submitKeyRef,
    onSubmitRef: refs.onSubmitRef,
    planModeEnabledRef: refs.planModeEnabledRef,
    onPlanModeChangeRef: refs.onPlanModeChangeRef,
    isSuggestionMenuOpenRef: refs.isSuggestionMenuOpenRef,
    keyboardShortcutsRef: refs.keyboardShortcutsRef,
  });
  const historyController = useHistoryKeymap({
    disabledRef: refs.disabledRef,
    isSuggestionMenuOpenRef: refs.isSuggestionMenuOpenRef,
    isReverseSearchOpenRef: refs.isReverseSearchOpenRef,
    getHistoryRef: refs.getHistoryRef,
    onOpenReverseSearchRef: refs.onOpenReverseSearchRef,
    onChangeRef: refs.onChangeRef,
    keyboardShortcutsRef: refs.keyboardShortcutsRef,
  });
  const isSyncingRef = useRef(false);
  const initialSyncDoneRef = useRef(false);
  const editor = useEditor({
    immediatelyRender: false,
    extensions: buildEditorExtensions({
      mentionSuggestion: opts.mentionSuggestion,
      slashSuggestion: opts.slashSuggestion,
      submitKeymap: SubmitKeymap,
      historyKeymap: historyController.extension,
    }),
    editorProps: buildEditorProps({
      planModeEnabled: opts.planModeEnabled,
      className: opts.className,
      onFocus: opts.onFocus,
      onBlur: opts.onBlur,
      onImagePasteRef: refs.onImagePasteRef,
    }),
    onUpdate: ({ editor: e }) => {
      if (isSyncingRef.current || !initialSyncDoneRef.current) return;
      const text = getMarkdownText(e);
      refs.onChangeRef.current(text);
      const sid = refs.sessionIdRef.current;
      if (sid) setChatDraftContent(sid, e.getJSON());
    },
    editable: !opts.disabled,
  });
  useSyncEditor({
    editor,
    disabled: opts.disabled,
    placeholder: opts.placeholder,
    sessionId: opts.sessionId,
    value: opts.value,
    isSyncingRef,
    initialSyncDoneRef,
    onChangeRef: refs.onChangeRef,
  });
  useEditorImperativeHandle(opts.ref, editor, opts.onChange, isSyncingRef);
  const applyHistoryEntry = useMemo(
    () => (index: number) => historyController.applyHistoryIndex(editor, index),
    [editor, historyController],
  );
  return { editor, applyHistoryEntry };
}

// ── Sync hook ─────────────────────────────────────────────────────

type SyncEditorOptions = {
  editor: ReturnType<typeof useEditor> | null;
  disabled: boolean;
  placeholder: string;
  sessionId: string | null;
  value: string;
  isSyncingRef: React.RefObject<boolean>;
  initialSyncDoneRef: React.RefObject<boolean>;
  onChangeRef: React.RefObject<(value: string) => void>;
};

function useSyncEditor({
  editor,
  disabled,
  placeholder,
  sessionId,
  value,
  isSyncingRef,
  initialSyncDoneRef,
  onChangeRef,
}: SyncEditorOptions) {
  // Sync disabled state
  useEffect(() => {
    if (editor) editor.setEditable(!disabled);
  }, [editor, disabled]);

  // Sync placeholder via editor.storage. The DynamicPlaceholder extension reads
  // from editor.storage.dynamicPlaceholder.text at decoration time.
  useEffect(() => {
    if (!editor) return;
    updateDynamicPlaceholder(editor, placeholder);
  }, [editor, placeholder]);

  // Reset sync flag when session changes
  const prevSyncSessionRef = useRef(sessionId);
  useEffect(() => {
    if (sessionId === prevSyncSessionRef.current) return;
    prevSyncSessionRef.current = sessionId;
    initialSyncDoneRef.current = false;
  }, [sessionId, initialSyncDoneRef]);

  // Sync value prop changes
  useEffect(() => {
    syncEditorValue({ editor, sessionId, value, isSyncingRef, initialSyncDoneRef, onChangeRef });
  }, [editor, value, sessionId, isSyncingRef, initialSyncDoneRef, onChangeRef]);
}

type SyncEditorValueOptions = {
  editor: ReturnType<typeof useEditor> | null;
  sessionId: string | null;
  value: string;
  isSyncingRef: React.RefObject<boolean>;
  initialSyncDoneRef: React.RefObject<boolean>;
  onChangeRef: React.RefObject<(value: string) => void>;
};

function syncEditorValue({
  editor,
  sessionId,
  value,
  isSyncingRef,
  initialSyncDoneRef,
  onChangeRef,
}: SyncEditorValueOptions) {
  if (!editor) return;

  if (!initialSyncDoneRef.current) {
    const sid = sessionId;
    if (sid) {
      const savedContent = getChatDraftContent(sid);
      if (savedContent) {
        isSyncingRef.current = true;
        editor.commands.setContent(savedContent as import("@tiptap/core").Content);
        isSyncingRef.current = false;
        initialSyncDoneRef.current = true;
        onChangeRef.current(getMarkdownText(editor));
        return;
      }
    }
  }

  if (value === "") {
    if (!editor.isEmpty) {
      isSyncingRef.current = true;
      editor.commands.clearContent();
      isSyncingRef.current = false;
    }
    initialSyncDoneRef.current = true;
    return;
  }

  const currentText = getMarkdownText(editor);
  if (currentText === value) {
    initialSyncDoneRef.current = true;
    return;
  }

  isSyncingRef.current = true;
  editor.commands.setContent(textToHtml(value));
  isSyncingRef.current = false;
  initialSyncDoneRef.current = true;
}

// ── Submit shortcut decision ────────────────────────────────────────

export type SubmitShortcutDecision = "consume-noop" | "submit" | "defer";

/** Pure decision for whether an Enter/Mod-Enter press in the chat input should
 *  submit the message, defer to the next ProseMirror handler (e.g. the slash/
 *  mention suggestion plugin or paragraph-split), or no-op while disabled.
 *  Kept pure so the keymap contract is unit-testable without mounting TipTap. */
export function decideSubmitShortcut(args: {
  pressed: "enter" | "mod-enter";
  disabled: boolean;
  submitKey: "enter" | "cmd_enter";
  isSuggestionMenuOpen: boolean;
}): SubmitShortcutDecision {
  if (args.disabled) return "consume-noop";
  if (args.pressed === "enter") {
    if (args.submitKey !== "enter") return "defer";
    if (args.isSuggestionMenuOpen) return "defer";
    return "submit";
  }
  if (args.submitKey !== "cmd_enter") return "defer";
  return "submit";
}

// ── Submit keymap hook ──────────────────────────────────────────────

function useSubmitKeymap(refs: {
  disabledRef: React.RefObject<boolean | undefined>;
  submitKeyRef: React.RefObject<"enter" | "cmd_enter">;
  onSubmitRef: React.RefObject<(() => void) | undefined>;
  planModeEnabledRef: React.RefObject<boolean>;
  onPlanModeChangeRef: React.RefObject<((enabled: boolean) => void) | undefined>;
  isSuggestionMenuOpenRef: React.RefObject<boolean>;
  keyboardShortcutsRef: React.RefObject<StoredShortcutOverrides | undefined>;
}) {
  const {
    disabledRef,
    submitKeyRef,
    onSubmitRef,
    planModeEnabledRef,
    onPlanModeChangeRef,
    isSuggestionMenuOpenRef,
    keyboardShortcutsRef,
  } = refs;
  return useMemo(() => {
    return Extension.create({
      name: "submitKeymap",
      addKeyboardShortcuts() {
        const run = (pressed: "enter" | "mod-enter") => {
          const decision = decideSubmitShortcut({
            pressed,
            disabled: !!disabledRef.current,
            submitKey: submitKeyRef.current ?? "cmd_enter",
            isSuggestionMenuOpen: isSuggestionMenuOpenRef.current,
          });
          if (decision === "consume-noop") return true;
          if (decision === "submit") {
            onSubmitRef.current?.();
            return true;
          }
          return false;
        };
        return {
          Enter: () => run("enter"),
          "Mod-Enter": () => run("mod-enter"),
        };
      },
      addProseMirrorPlugins() {
        return [
          new Plugin({
            key: new PluginKey("planModeToggle"),
            props: {
              handleKeyDown: (_view, event) => {
                const shortcut = getShortcut("TOGGLE_PLAN_MODE", keyboardShortcutsRef.current);
                if (matchesShortcut(event, shortcut)) {
                  onPlanModeChangeRef.current?.(!planModeEnabledRef.current);
                  return true;
                }
                return false;
              },
            },
          }),
        ];
      },
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
}

// ── Imperative handle hook ──────────────────────────────────────────

function useEditorImperativeHandle(
  ref: React.ForwardedRef<TipTapInputHandle>,
  editor: ReturnType<typeof useEditor> | null,
  onChange: (value: string) => void,
  isSyncingRef: React.RefObject<boolean>,
) {
  useImperativeHandle(
    ref,
    () => ({
      focus: () => editor?.commands.focus(),
      blur: () => editor?.commands.blur(),
      getSelectionStart: () => editor?.state.selection.from ?? 0,
      getValue: () => (editor ? getMarkdownText(editor) : ""),
      setValue: (v: string) => {
        if (!editor) return;
        isSyncingRef.current = true;
        if (v === "") {
          editor.commands.clearContent();
        } else {
          editor.commands.setContent(textToHtml(v));
        }
        isSyncingRef.current = false;
        onChange(v);
      },
      clear: () => {
        if (!editor) return;
        isSyncingRef.current = true;
        editor.commands.clearContent();
        isSyncingRef.current = false;
        onChange("");
      },
      getTextareaElement: () => editor?.view.dom ?? null,
      insertText: (text: string, from: number, to: number) => {
        if (!editor) return;
        editor.chain().focus().insertContentAt({ from, to }, text).run();
      },
      getMentions: () => {
        if (!editor) return [];
        const mentions: ContextFile[] = [];
        editor.state.doc.descendants((node) => {
          if (node.type.name === "contextMention") {
            const { kind, path, label } = node.attrs;
            if (kind === "file") mentions.push({ path, name: label, pinned: false });
            else if (kind === "prompt") mentions.push({ path, name: label, pinned: false });
            else if (kind === "plan") mentions.push({ path, name: label, pinned: false });
          }
        });
        return mentions;
      },
      getTaskMentions: () => {
        if (!editor) return [];
        const seen = new Set<string>();
        const mentions: TaskMentionData[] = [];
        editor.state.doc.descendants((node) => {
          if (node.type.name !== "contextMention") return;
          const { kind, label, taskId, workflowId, workflowStepId, taskState } = node.attrs;
          if (kind !== "task" || !taskId || !workflowId || !workflowStepId || seen.has(taskId))
            return;
          seen.add(taskId);
          mentions.push({
            taskId,
            title: label ?? taskId,
            workflowId,
            workflowStepId,
            state: taskState ?? null,
          });
        });
        return mentions;
      },
    }),
    [editor, onChange, isSyncingRef],
  );
}
