"use client";

import { useRef, useImperativeHandle, useEffect, useLayoutEffect, useMemo } from "react";
import { useEditor, ReactNodeViewRenderer } from "@tiptap/react";
import Document from "@tiptap/extension-document";
import Paragraph from "@tiptap/extension-paragraph";
import Text from "@tiptap/extension-text";
import HardBreak from "@tiptap/extension-hard-break";
import History from "@tiptap/extension-history";
import Placeholder from "@tiptap/extension-placeholder";
import Code from "@tiptap/extension-code";
import CodeBlockLowlight from "@tiptap/extension-code-block-lowlight";
import { common, createLowlight } from "lowlight";
import { Extension } from "@tiptap/core";
import { cn } from "@/lib/utils";
import { getChatDraftContent, setChatDraftContent } from "@/lib/local-storage";
import { getMarkdownText, escapeHtml, handleEditorPaste } from "./tiptap-helpers";
import { CodeBlockView } from "./tiptap-code-block-view";
import { ContextMention } from "./tiptap-mention-extension";
import type { ContextFile } from "@/lib/state/context-files-store";

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
};

const lowlightInstance = createLowlight(common);

type UseTipTapEditorOptions = {
  value: string;
  onChange: (value: string) => void;
  onSubmit?: () => void;
  placeholder: string;
  disabled: boolean;
  className?: string;
  planModeEnabled: boolean;
  submitKey: "enter" | "cmd_enter";
  onFocus?: () => void;
  onBlur?: () => void;
  sessionId: string | null;
  onImagePaste?: (files: File[]) => void;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  mentionSuggestion: any;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  slashSuggestion: any;
  ref: React.ForwardedRef<TipTapInputHandle>;
};

function useTipTapRefs(opts: UseTipTapEditorOptions) {
  const onSubmitRef = useRef(opts.onSubmit);
  const submitKeyRef = useRef(opts.submitKey);
  const disabledRef = useRef(opts.disabled);
  const onChangeRef = useRef(opts.onChange);
  const onImagePasteRef = useRef(opts.onImagePaste);
  const sessionIdRef = useRef(opts.sessionId);
  useLayoutEffect(() => {
    onSubmitRef.current = opts.onSubmit;
    submitKeyRef.current = opts.submitKey;
    disabledRef.current = opts.disabled;
    onChangeRef.current = opts.onChange;
    onImagePasteRef.current = opts.onImagePaste;
    sessionIdRef.current = opts.sessionId;
  });
  return { onSubmitRef, submitKeyRef, disabledRef, onChangeRef, onImagePasteRef, sessionIdRef };
}

export function useTipTapEditor(opts: UseTipTapEditorOptions) {
  const { value, onChange, placeholder, disabled, className, planModeEnabled, onFocus, onBlur, sessionId, mentionSuggestion, slashSuggestion, ref } = opts;
  const refs = useTipTapRefs(opts);
  const SubmitKeymap = useSubmitKeymap(refs.disabledRef, refs.submitKeyRef, refs.onSubmitRef);
  const isSyncingRef = useRef(false);
  const initialSyncDoneRef = useRef(false);
  const editor = useEditor({
    immediatelyRender: false,
    extensions: [
      Document, Paragraph, Text, HardBreak, History, Code,
      CodeBlockLowlight.extend({
        addNodeView() { return ReactNodeViewRenderer(CodeBlockView); },
      }).configure({ lowlight: lowlightInstance }),
      Placeholder.configure({ placeholder }),
      ContextMention.configure({ suggestions: [mentionSuggestion, slashSuggestion] }),
      SubmitKeymap,
    ],
    editorProps: {
      attributes: {
        class: cn(
          "w-full h-full resize-none bg-transparent px-2 py-2 overflow-y-auto",
          "text-sm leading-relaxed", "placeholder:text-muted-foreground",
          "focus:outline-none", "disabled:cursor-not-allowed disabled:opacity-50",
          planModeEnabled && "border-primary/40", className,
        ),
      },
      handlePaste: (view, event) => handleEditorPaste(view, event, refs.onImagePasteRef),
      handleDOMEvents: {
        focus: () => { onFocus?.(); return false; },
        blur: () => { onBlur?.(); return false; },
      },
    },
    onUpdate: ({ editor: e }) => {
      if (isSyncingRef.current || !initialSyncDoneRef.current) return;
      const text = getMarkdownText(e);
      refs.onChangeRef.current(text);
      const sid = refs.sessionIdRef.current;
      if (sid) setChatDraftContent(sid, e.getJSON());
    },
    editable: !disabled,
  });
  useSyncEditor({ editor, disabled, placeholder, sessionId, value, isSyncingRef, initialSyncDoneRef, onChangeRef: refs.onChangeRef });
  useEditorImperativeHandle(ref, editor, onChange, isSyncingRef);
  return editor;
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

  // Sync placeholder
  useEffect(() => {
    if (!editor) return;
    editor.extensionManager.extensions.forEach((ext) => {
      if (ext.name === "placeholder") {
        ext.options.placeholder = placeholder;
        editor.view.dispatch(editor.state.tr);
      }
    });
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
  editor.commands.setContent(`<p>${escapeHtml(value)}</p>`);
  isSyncingRef.current = false;
  initialSyncDoneRef.current = true;
}

// ── Submit keymap hook ──────────────────────────────────────────────

function useSubmitKeymap(
  disabledRef: React.RefObject<boolean | undefined>,
  submitKeyRef: React.RefObject<"enter" | "cmd_enter">,
  onSubmitRef: React.RefObject<(() => void) | undefined>,
) {
  return useMemo(() => {
    return Extension.create({
      name: "submitKeymap",
      addKeyboardShortcuts() {
        return {
          Enter: () => {
            if (disabledRef.current) return true;
            if (submitKeyRef.current === "enter") {
              onSubmitRef.current?.();
              return true;
            }
            return false;
          },
          "Mod-Enter": () => {
            if (disabledRef.current) return true;
            if (submitKeyRef.current === "cmd_enter") {
              onSubmitRef.current?.();
              return true;
            }
            return false;
          },
        };
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
          editor.commands.setContent(`<p>${escapeHtml(v)}</p>`);
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
          }
        });
        return mentions;
      },
    }),
    [editor, onChange, isSyncingRef],
  );
}
