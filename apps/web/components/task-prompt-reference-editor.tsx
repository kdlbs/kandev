"use client";

import { useEditor, EditorContent } from "@tiptap/react";
import Document from "@tiptap/extension-document";
import HardBreak from "@tiptap/extension-hard-break";
import History from "@tiptap/extension-history";
import Paragraph from "@tiptap/extension-paragraph";
import Placeholder from "@tiptap/extension-placeholder";
import Text from "@tiptap/extension-text";
import type { Editor } from "@tiptap/core";
import { Fragment, type Node as ProseMirrorNode } from "@tiptap/pm/model";
import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useMemo,
  useRef,
  type KeyboardEvent,
  type ClipboardEvent,
  type MutableRefObject,
} from "react";
import { MentionMenu } from "@/components/task/chat/mention-menu";
import type { RichTextInputHandle } from "@/components/task/chat/rich-text-input";
import {
  useStablePromptMentionNames,
  splitMarkdownPromptMentionSegments,
} from "@/components/task/chat/messages/prompt-mention-components";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";
import { detectMentionTrigger } from "@/hooks/use-inline-mention";
import { useTaskCreatePromptMentionForInput } from "@/hooks/use-task-create-prompt-mention";
import { cn } from "@/lib/utils";
import {
  buildTaskPromptDocument,
  serializeTaskPromptDocument,
  type TaskPromptDocument,
} from "@/lib/prompts/task-prompt-document";
import { TaskPromptReference } from "./task-prompt-reference-node";

type TaskPromptReferenceEditorProps = {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  autoFocus?: boolean;
  className?: string;
  onKeyDown?: (event: KeyboardEvent<HTMLElement>) => void;
  onPaste?: (event: ClipboardEvent<HTMLElement>) => void;
  onFocus?: () => void;
  onBlur?: () => void;
};

export const TaskPromptReferenceEditor = forwardRef<
  RichTextInputHandle,
  TaskPromptReferenceEditorProps
>(function TaskPromptReferenceEditor(
  {
    value,
    onChange,
    placeholder = "",
    disabled = false,
    autoFocus = false,
    className,
    onKeyDown,
    onPaste,
    onFocus,
    onBlur,
  },
  ref,
) {
  const { prompts } = useCustomPrompts();
  const promptNames = useStablePromptMentionNames(prompts.map((prompt) => prompt.name));
  const valueRef = useRef(value);
  const onChangeRef = useRef(onChange);
  const onKeyDownRef = useRef(onKeyDown);
  const onPasteRef = useRef(onPaste);
  const onFocusRef = useRef(onFocus);
  const onBlurRef = useRef(onBlur);
  const syncingRef = useRef(false);
  const mentionRef = useRef<ReturnType<typeof useTaskCreatePromptMentionForInput> | null>(null);
  const editorRef = useRef<Editor | null>(null);

  useLayoutEffect(() => {
    valueRef.current = value;
    onChangeRef.current = onChange;
    onKeyDownRef.current = onKeyDown;
    onPasteRef.current = onPaste;
    onFocusRef.current = onFocus;
    onBlurRef.current = onBlur;
  });

  const initialContent = useMemo(
    () => buildTaskPromptDocument(value, promptNames),
    [promptNames, value],
  );

  const editor = useTaskPromptTiptap({
    value,
    placeholder,
    disabled,
    className,
    initialContent,
    promptNames,
    valueRef,
    mentionRef,
    editorRef,
    onKeyDownRef,
    onPasteRef,
    onFocusRef,
    onBlurRef,
    syncingRef,
  });

  const editorInputRef = useRef<RichTextInputHandle | null>(null);
  const mention = useTaskCreatePromptMentionForInput({
    inputRef: editorInputRef,
    value,
    onChange,
    promptInsertMode: "reference",
  });
  useLayoutEffect(() => {
    mentionRef.current = mention;
  }, [mention]);

  const handleMentionKeyDownCapture = useCallback(
    (event: KeyboardEvent<HTMLDivElement>) => {
      if (event.key === "Escape" && mention.isOpen) {
        event.preventDefault();
        event.stopPropagation();
        mention.closeMenu();
        requestAnimationFrame(() => editorInputRef.current?.focus());
        return;
      }
      if (event.defaultPrevented) return;
      mention.handleKeyDown(event as unknown as KeyboardEvent<HTMLElement>);
    },
    [mention.closeMenu, mention.handleKeyDown, mention.isOpen],
  );

  const createInputHandle = useCallback(
    () => createEditorInputHandle(editor, promptNames, valueRef, onChangeRef, syncingRef),
    [editor, promptNames],
  );

  useImperativeHandle(editorInputRef, createInputHandle, [createInputHandle]);
  useImperativeHandle(ref, createInputHandle, [createInputHandle]);

  useSyncPromptReferenceContent(editor, value, promptNames, valueRef, syncingRef);
  usePromptReferenceAutoFocus(editor, autoFocus);

  return (
    <TaskPromptReferenceEditorView
      editor={editor}
      mention={mention}
      onKeyDownCapture={handleMentionKeyDownCapture}
    />
  );
});

type TaskPromptTiptapOptions = {
  value: string;
  placeholder: string;
  disabled: boolean;
  className?: string;
  initialContent: TaskPromptDocument;
  promptNames: readonly string[];
  valueRef: MutableRefObject<string>;
  mentionRef: MutableRefObject<ReturnType<typeof useTaskCreatePromptMentionForInput> | null>;
  editorRef: MutableRefObject<Editor | null>;
  onKeyDownRef: MutableRefObject<((event: KeyboardEvent<HTMLElement>) => void) | undefined>;
  onPasteRef: MutableRefObject<((event: ClipboardEvent<HTMLElement>) => void) | undefined>;
  onFocusRef: MutableRefObject<(() => void) | undefined>;
  onBlurRef: MutableRefObject<(() => void) | undefined>;
  syncingRef: MutableRefObject<boolean>;
};

function useTaskPromptTiptap({
  value,
  placeholder,
  disabled,
  className,
  initialContent,
  promptNames,
  valueRef,
  mentionRef,
  editorRef,
  onKeyDownRef,
  onPasteRef,
  onFocusRef,
  onBlurRef,
  syncingRef,
}: TaskPromptTiptapOptions) {
  return useEditor({
    immediatelyRender: false,
    extensions: [
      Document,
      Paragraph,
      Text,
      HardBreak,
      History,
      Placeholder.configure({ placeholder }),
      TaskPromptReference,
    ],
    content: initialContent,
    editable: !disabled,
    editorProps: {
      attributes: {
        "data-testid": "task-description-input",
        "data-prompt-reference-editor": "true",
        "data-task-description-value": value,
        class: cn(
          "min-w-0 max-w-full whitespace-pre-wrap break-words px-2 py-2 text-[13px] leading-relaxed outline-none",
          "min-h-[96px] max-h-[240px] overflow-y-auto",
          "placeholder:text-muted-foreground",
          className,
        ),
      },
      handleKeyDown: (_view, event) => {
        const wasDefaultPrevented = event.defaultPrevented;
        mentionRef.current?.handleKeyDown(event as unknown as KeyboardEvent<HTMLElement>);
        if (!event.defaultPrevented) {
          onKeyDownRef.current?.(event as unknown as KeyboardEvent<HTMLElement>);
        }
        return !wasDefaultPrevented && event.defaultPrevented;
      },
      handleDOMEvents: {
        paste: (_view, event) => {
          onPasteRef.current?.(event as unknown as ClipboardEvent<HTMLElement>);
          return event.defaultPrevented;
        },
        focus: () => {
          onFocusRef.current?.();
          return false;
        },
        blur: () => {
          const currentEditor = editorRef.current;
          if (currentEditor && !currentEditor.isDestroyed) {
            reconcilePromptReferences(currentEditor, promptNames);
          }
          onBlurRef.current?.();
          return false;
        },
      },
    },
    onCreate: ({ editor: nextEditor }) => {
      editorRef.current = nextEditor;
    },
    onDestroy: () => {
      editorRef.current = null;
    },
    onUpdate: ({ editor: nextEditor }) => {
      if (syncingRef.current) return;
      const nextValue = serializeEditor(nextEditor);
      valueRef.current = nextValue;
      const cursor = documentPositionToTextOffset(nextEditor, nextEditor.state.selection.from);
      mentionRef.current?.handleChange(nextValue, cursor);
      requestAnimationFrame(() => {
        if (nextEditor.isDestroyed || nextEditor.view.composing) return;
        if (!detectMentionTrigger(nextValue, cursor)) {
          reconcilePromptReferences(nextEditor, promptNames);
        }
      });
    },
  });
}

type TaskPromptReferenceEditorViewProps = {
  editor: Editor | null;
  mention: ReturnType<typeof useTaskCreatePromptMentionForInput>;
  onKeyDownCapture: (event: KeyboardEvent<HTMLDivElement>) => void;
};

function TaskPromptReferenceEditorView({
  editor,
  mention,
  onKeyDownCapture,
}: TaskPromptReferenceEditorViewProps) {
  return (
    <>
      <div className="min-w-0 max-w-full" onKeyDownCapture={onKeyDownCapture}>
        <EditorContent editor={editor} />
      </div>
      <MentionMenu
        isOpen={mention.isOpen}
        isLoading={mention.isLoading}
        position={mention.position}
        items={mention.items}
        query={mention.query}
        selectedIndex={mention.selectedIndex}
        onSelect={mention.handleSelect}
        onClose={mention.closeMenu}
        setSelectedIndex={mention.setSelectedIndex}
      />
    </>
  );
}

function useSyncPromptReferenceContent(
  editor: Editor | null,
  value: string,
  promptNames: readonly string[],
  valueRef: MutableRefObject<string>,
  syncingRef: MutableRefObject<boolean>,
) {
  useEffect(() => {
    if (!editor || editor.isDestroyed) return;
    editor.view.dom.setAttribute("data-task-description-value", value);
    const currentValue = serializeEditor(editor);
    if (currentValue === value) return;
    syncingRef.current = true;
    editor.commands.setContent(buildTaskPromptDocument(value, promptNames), { emitUpdate: false });
    syncingRef.current = false;
    valueRef.current = value;
  }, [editor, promptNames, value, valueRef, syncingRef]);

  useEffect(() => {
    if (!editor || editor.isDestroyed) return;
    reconcilePromptReferences(editor, promptNames);
  }, [editor, promptNames]);
}

function usePromptReferenceAutoFocus(editor: Editor | null, autoFocus: boolean) {
  useEffect(() => {
    if (!editor || !autoFocus) return;
    editor.commands.focus("end");
  }, [autoFocus, editor]);
}

function serializeEditor(editor: Editor): string {
  return serializeTaskPromptDocument(editor.getJSON() as TaskPromptDocument);
}

function createEditorInputHandle(
  editor: Editor | null,
  promptNames: readonly string[],
  valueRef: MutableRefObject<string>,
  onChangeRef: MutableRefObject<(value: string) => void>,
  syncingRef: MutableRefObject<boolean>,
): RichTextInputHandle {
  return {
    focus: () => editor?.commands.focus(),
    blur: () => editor?.commands.blur(),
    getSelectionStart: () =>
      editor ? documentPositionToTextOffset(editor, editor.state.selection.from) : 0,
    getSelectionEnd: () =>
      editor ? documentPositionToTextOffset(editor, editor.state.selection.to) : 0,
    setSelectionRange: (start, end) => {
      if (!editor) return;
      editor
        .chain()
        .focus()
        .setTextSelection({
          from: textOffsetToDocumentPosition(editor, start),
          to: textOffsetToDocumentPosition(editor, end),
        })
        .run();
    },
    getCaretRect: () => {
      if (!editor) return null;
      const position = textOffsetToDocumentPosition(
        editor,
        documentPositionToTextOffset(editor, editor.state.selection.from),
      );
      const rect = editor.view.coordsAtPos(position);
      return new DOMRect(rect.left, rect.top, rect.right - rect.left, rect.bottom - rect.top);
    },
    getValue: () => (editor ? serializeEditor(editor) : valueRef.current),
    setValue: (nextValue) => {
      if (!editor) {
        valueRef.current = nextValue;
        onChangeRef.current(nextValue);
        return;
      }
      syncingRef.current = true;
      editor.commands.setContent(buildTaskPromptDocument(nextValue, promptNames), {
        emitUpdate: false,
      });
      syncingRef.current = false;
      valueRef.current = nextValue;
      onChangeRef.current(nextValue);
    },
    insertText: (text, from, to) => {
      if (!editor) return;
      editor
        .chain()
        .focus()
        .insertContentAt(
          {
            from: textOffsetToDocumentPosition(editor, from),
            to: textOffsetToDocumentPosition(editor, to),
          },
          text,
        )
        .run();
      reconcilePromptReferences(editor, promptNames);
    },
    getTextareaElement: () => editor?.view.dom ?? null,
  };
}

function reconcilePromptReferences(editor: Editor, promptNames: readonly string[]) {
  if (editor.isDestroyed || editor.view.composing) return;
  const preparedNames = [...promptNames];
  let transaction = editor.state.tr;
  let changed = false;

  for (let index = 0, offset = 0; index < editor.state.doc.childCount; index += 1) {
    const block = editor.state.doc.child(index);
    if (block.type.name !== "paragraph") {
      offset += block.nodeSize;
      continue;
    }
    const blockText = serializeInlineContent(block);
    const segments = splitMarkdownPromptMentionSegments(blockText, preparedNames);
    const replacement = segmentsToNodes(editor, segments);
    if (sameInlineContent(block, replacement)) {
      offset += block.nodeSize;
      continue;
    }
    const from = offset + 1;
    transaction = transaction.replaceWith(
      from,
      from + block.content.size,
      Fragment.fromArray(replacement),
    );
    changed = true;
    offset += block.nodeSize;
  }

  if (changed) {
    editor.view.dispatch(transaction.setMeta("addToHistory", false));
  }
}

function serializeInlineContent(block: ProseMirrorNode): string {
  return block.content.content
    .map((node) => {
      if (node.type.name === "promptReference") return String(node.attrs.value ?? "");
      if (node.type.name === "hardBreak") return "\n";
      return node.text ?? "";
    })
    .join("");
}

function segmentsToNodes(
  editor: Editor,
  segments: ReturnType<typeof splitMarkdownPromptMentionSegments>,
): ProseMirrorNode[] {
  const nodes: ProseMirrorNode[] = [];
  for (const segment of segments) {
    if (segment.kind === "prompt") {
      nodes.push(
        editor.schema.nodes.promptReference.create({ name: segment.name, value: segment.value }),
      );
      continue;
    }
    appendTextNodes(editor, nodes, segment.value);
  }
  return nodes;
}

function appendTextNodes(editor: Editor, nodes: ProseMirrorNode[], value: string) {
  let start = 0;
  for (let index = 0; index <= value.length; index += 1) {
    if (index < value.length && value[index] !== "\n") continue;
    const text = value.slice(start, index);
    if (text) nodes.push(editor.schema.text(text));
    if (index < value.length) nodes.push(editor.schema.nodes.hardBreak.create());
    start = index + 1;
  }
}

function sameInlineContent(block: ProseMirrorNode, replacement: ProseMirrorNode[]) {
  if (block.childCount !== replacement.length) return false;
  return replacement.every((next, index) => next.eq(block.child(index)));
}

function documentPositionToTextOffset(editor: Editor, position: number): number {
  let textOffset = 0;
  let childOffset = 0;
  for (let index = 0; index < editor.state.doc.childCount; index += 1) {
    const block = editor.state.doc.child(index);
    const blockStart = childOffset + 1;
    const blockOffset = blockPositionToTextOffset(block, blockStart, position);
    if (blockOffset !== null) return textOffset + blockOffset;
    textOffset += serializeInlineContent(block).length;
    childOffset += block.nodeSize;
    if (index < editor.state.doc.childCount - 1) textOffset += 1;
  }
  return textOffset;
}

function blockPositionToTextOffset(
  block: ProseMirrorNode,
  blockStart: number,
  position: number,
): number | null {
  if (position > blockStart + block.content.size) return null;
  let inlinePosition = blockStart;
  let inlineOffset = 0;
  for (let index = 0; index < block.childCount; index += 1) {
    const child = block.child(index);
    const length = promptNodeTextLength(child);
    const childEnd = inlinePosition + child.nodeSize;
    if (position <= childEnd) {
      return (
        inlineOffset + nodePositionToTextOffset(child, position, inlinePosition, childEnd, length)
      );
    }
    inlinePosition = childEnd;
    inlineOffset += length;
  }
  return inlineOffset;
}

function nodePositionToTextOffset(
  node: ProseMirrorNode,
  position: number,
  nodeStart: number,
  nodeEnd: number,
  textLength: number,
): number {
  if (node.isText) return Math.max(0, position - nodeStart);
  return position === nodeEnd ? textLength : 0;
}

function textOffsetToDocumentPosition(editor: Editor, target: number): number {
  let textOffset = 0;
  let childOffset = 0;
  for (let index = 0; index < editor.state.doc.childCount; index += 1) {
    const block = editor.state.doc.child(index);
    const blockStart = childOffset + 1;
    const blockText = serializeInlineContent(block);
    const blockEndOffset = textOffset + blockText.length;
    if (target <= blockEndOffset) {
      return inlineTextOffsetToPosition(block, blockStart, target - textOffset);
    }
    textOffset = blockEndOffset;
    childOffset += block.nodeSize;
    if (index < editor.state.doc.childCount - 1) {
      if (target === textOffset) return blockStart + block.content.size;
      textOffset += 1;
    }
  }
  return editor.state.doc.content.size;
}

function inlineTextOffsetToPosition(block: ProseMirrorNode, blockStart: number, target: number) {
  let position = blockStart;
  let offset = 0;
  for (let index = 0; index < block.childCount; index += 1) {
    const child = block.child(index);
    const length = promptNodeTextLength(child);
    if (target <= offset + length) {
      if (child.isText) return position + Math.max(0, target - offset);
      return target === offset + length ? position + child.nodeSize : position;
    }
    offset += length;
    position += child.nodeSize;
  }
  return position;
}

function promptNodeTextLength(node: ProseMirrorNode): number {
  if (node.type.name === "promptReference") return String(node.attrs.value ?? "").length;
  if (node.type.name === "hardBreak") return 1;
  return node.text?.length ?? 0;
}
