"use client";

import { useEffect, useRef, useState } from "react";
import {
  CommentModeController,
  CommentsModel,
  CommentsView,
  createDefaultMonacoSyntaxHighlighter,
  EditorController,
  EditorModel,
  EditorView,
  LocalHistoryStrategy,
  OffsetRange,
  Selection,
  StringEdit,
  StringValue,
  type Comment,
  type CommentSubmission,
  type EditorViewOptions,
  type IMonarchApi,
  type MonacoSyntaxHighlighter,
  type TableAstNode,
} from "@vscode/markdown-editor";
import { language as cssLanguage } from "monaco-editor/esm/vs/basic-languages/css/css.js";
import { language as htmlLanguage } from "monaco-editor/esm/vs/basic-languages/html/html.js";
import { language as javascriptLanguage } from "monaco-editor/esm/vs/basic-languages/javascript/javascript.js";
import { language as pythonLanguage } from "monaco-editor/esm/vs/basic-languages/python/python.js";
import { language as rustLanguage } from "monaco-editor/esm/vs/basic-languages/rust/rust.js";
import { language as shellLanguage } from "monaco-editor/esm/vs/basic-languages/shell/shell.js";
import { language as typescriptLanguage } from "monaco-editor/esm/vs/basic-languages/typescript/typescript.js";
import { language as yamlLanguage } from "monaco-editor/esm/vs/basic-languages/yaml/yaml.js";
import { compile } from "monaco-editor/esm/vs/editor/standalone/common/monarch/monarchCompile.js";
import { MonarchTokenizer } from "monaco-editor/esm/vs/editor/standalone/common/monarch/monarchLexer.js";
import "@vscode/markdown-editor/editor.css";
import { insertMarkdownTableColumn, insertMarkdownTableRow } from "./markdown-table-edit";
import {
  HybridMarkdownTableEdgeChrome,
  type HybridMarkdownTableEdgeChromeProps,
} from "./hybrid-markdown-table-edge";
import "./hybrid-markdown-editor.css";

export type MarkdownSourceRange = {
  start: number;
  endExclusive: number;
};

export type MarkdownGutterMarker = MarkdownSourceRange & {
  kind: "added" | "modified" | "deleted";
};

export type MarkdownComment = MarkdownSourceRange & {
  id: string;
  body: string;
  author?: string;
  createdAt?: number;
};

export type HybridMarkdownEditorProps = {
  content: string;
  readOnly?: boolean;
  baseline?: string;
  gutterMarkers?: readonly MarkdownGutterMarker[];
  comments?: readonly MarkdownComment[];
  className?: string;
  onChange: (content: string) => void;
  onOpenLink?: (url: string) => boolean | void;
  onComment?: (comment: MarkdownCommentSubmission) => void;
  onError?: (error: unknown) => void;
  onSourceFallback?: () => void;
};

export type MarkdownCommentSubmission = {
  text: string;
  start: number;
  endExclusive: number;
};

type EditorLifecycle = {
  model: EditorModel;
  view: EditorView;
  controller: EditorController;
  commentsModel: CommentsModel;
  commentsView: CommentsView;
  commentMode: CommentModeController;
  historyStrategy: LocalHistoryStrategy;
  syntaxHighlighter: MonacoSyntaxHighlighter;
  sourceEditSubscription: { dispose: () => void };
};

type EditorCallbackRefs = {
  onChange: { current: (content: string) => void };
  onOpenLink: { current: ((url: string) => boolean | void) | undefined };
  onComment: { current: ((comment: MarkdownCommentSubmission) => void) | undefined };
  onError: { current: ((error: unknown) => void) | undefined };
  onSourceFallback: { current: (() => void) | undefined };
};

type MutableRef<T> = { current: T };

export function HybridMarkdownEditor({
  content,
  readOnly = false,
  baseline,
  gutterMarkers = [],
  comments = [],
  className,
  onChange,
  onOpenLink,
  onComment,
  onError,
  onSourceFallback,
}: HybridMarkdownEditorProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const lifecycleRef = useRef<EditorLifecycle | null>(null);
  const canonicalContentRef = useRef(content);
  const callbackRefs = useRef<EditorCallbackRefs>({
    onChange: { current: onChange },
    onOpenLink: { current: onOpenLink },
    onComment: { current: onComment },
    onError: { current: onError },
    onSourceFallback: { current: onSourceFallback },
  }).current;

  callbackRefs.onChange.current = onChange;
  callbackRefs.onOpenLink.current = onOpenLink;
  callbackRefs.onComment.current = onComment;
  callbackRefs.onError.current = onError;
  callbackRefs.onSourceFallback.current = onSourceFallback;

  useHybridEditorLifecycle({
    rootRef,
    lifecycleRef,
    canonicalContentRef,
    callbackRefs,
    content,
    readOnly,
    baseline,
    gutterMarkers,
    comments,
  });
  useHybridEditorContentSync({ lifecycleRef, canonicalContentRef, callbackRefs, content });
  useHybridEditorOptionsSync({
    lifecycleRef,
    callbackRefs,
    readOnly,
    baseline,
    gutterMarkers,
    comments,
  });
  const tableControlsHost = useActiveTableControlsHost(rootRef, !readOnly, lifecycleRef);

  return (
    <>
      <div
        ref={rootRef}
        className={
          className
            ? `kandev-hybrid-markdown-editor-root ${className}`
            : "kandev-hybrid-markdown-editor-root"
        }
        data-testid="hybrid-markdown-editor"
      />
      <HybridMarkdownTableEdgeChrome
        host={tableControlsHost?.host ?? null}
        tableKey={tableControlsHost?.tableKey ?? null}
        onInsertRow={(rowIndex) => applyActiveTableEdit(lifecycleRef, "row", rowIndex)}
        onInsertColumn={(columnIndex) => applyActiveTableEdit(lifecycleRef, "column", columnIndex)}
      />
    </>
  );
}

type ActiveTableControlsHost = Pick<HybridMarkdownTableEdgeChromeProps, "host" | "tableKey">;

const ACTIVE_TABLE_EDGE_WRAPPER_CLASS = "kandev-markdown-table-edge-wrapper";

type ActiveTableHostWithWrapper = ActiveTableControlsHost & {
  wrapper: HTMLElement;
};

function ensureActiveTableHost(
  root: HTMLDivElement,
  lifecycleRef: MutableRef<EditorLifecycle | null>,
): ActiveTableHostWithWrapper | null {
  const activeTable = root.querySelector<HTMLElement>(".md-table.md-block-active");
  const wrapper = activeTable?.closest<HTMLElement>(".md-table-wrapper");
  if (!wrapper || !root.contains(wrapper)) return null;

  const existing = Array.from(wrapper.children).find(
    (child): child is HTMLDivElement =>
      child instanceof HTMLDivElement && child.dataset.kandevTableControls === "true",
  );
  const host = existing ?? document.createElement("div");
  if (!existing) {
    host.dataset.kandevTableControls = "true";
    host.className = "kandev-markdown-table-controls-host";
    wrapper.prepend(host);
  }

  const activeBlock = lifecycleRef.current?.model.activeBlock.get();
  const range =
    activeBlock?.kind === "table" && lifecycleRef.current
      ? findActiveTableRange(lifecycleRef.current.model, activeBlock as TableAstNode)
      : undefined;
  return { host, tableKey: range ? `table-${range.start}` : "active-table", wrapper };
}

function useActiveTableControlsHost(
  rootRef: MutableRef<HTMLDivElement | null>,
  enabled: boolean,
  lifecycleRef: MutableRef<EditorLifecycle | null>,
): ActiveTableControlsHost | null {
  const [hostState, setHostState] = useState<ActiveTableControlsHost | null>(null);

  useEffect(() => {
    const root = rootRef.current;
    if (!root || !enabled) {
      setHostState(null);
      return;
    }

    let currentHost: HTMLDivElement | null = null;
    let currentWrapper: HTMLElement | null = null;
    const reconcile = () => {
      const next = ensureActiveTableHost(root, lifecycleRef);
      if (!next) {
        currentHost?.remove();
        currentWrapper?.classList.remove(ACTIVE_TABLE_EDGE_WRAPPER_CLASS);
        currentHost = null;
        currentWrapper = null;
        setHostState(null);
        return;
      }

      if (currentWrapper && currentWrapper !== next.wrapper) {
        currentWrapper.classList.remove(ACTIVE_TABLE_EDGE_WRAPPER_CLASS);
      }
      if (currentHost && currentHost !== next.host) currentHost.remove();
      if (!next.wrapper.classList.contains(ACTIVE_TABLE_EDGE_WRAPPER_CLASS)) {
        next.wrapper.classList.add(ACTIVE_TABLE_EDGE_WRAPPER_CLASS);
      }
      currentHost = next.host;
      currentWrapper = next.wrapper;
      setHostState((current) =>
        current?.host === next.host && current.tableKey === next.tableKey
          ? current
          : { host: next.host, tableKey: next.tableKey },
      );
    };

    reconcile();
    const observer = new MutationObserver(reconcile);
    observer.observe(root, { attributes: true, childList: true, subtree: true });
    return () => {
      observer.disconnect();
      currentHost?.remove();
      currentWrapper?.classList.remove(ACTIVE_TABLE_EDGE_WRAPPER_CLASS);
    };
  }, [enabled, lifecycleRef, rootRef]);

  return hostState;
}

type LifecycleHookOptions = {
  rootRef: MutableRef<HTMLDivElement | null>;
  lifecycleRef: MutableRef<EditorLifecycle | null>;
  canonicalContentRef: MutableRef<string>;
  callbackRefs: EditorCallbackRefs;
  content: string;
  readOnly: boolean;
  baseline: string | undefined;
  gutterMarkers: readonly MarkdownGutterMarker[];
  comments: readonly MarkdownComment[];
};

function useHybridEditorLifecycle({
  rootRef,
  lifecycleRef,
  canonicalContentRef,
  callbackRefs,
  content,
  readOnly,
  baseline,
  gutterMarkers,
  comments,
}: LifecycleHookOptions): void {
  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;

    let disposed = false;
    let lifecycle: EditorLifecycle | null = null;
    const fail = (error: unknown) => {
      if (disposed) return;
      disposed = true;
      disposeEditorLifecycle(lifecycle);
      lifecycle = null;
      lifecycleRef.current = null;
      callbackRefs.onError.current?.(error);
      callbackRefs.onSourceFallback.current?.();
    };

    try {
      lifecycle = createEditorLifecycle({
        root,
        content,
        readOnly,
        baseline,
        gutterMarkers,
        comments,
        onOpenLink: (url) => {
          const handler = callbackRefs.onOpenLink.current;
          if (!handler) return false;
          return handler(url) === false ? false : undefined;
        },
        onComment: (submission) => callbackRefs.onComment.current?.(submission),
        onSourceEdit: (nextContent) => {
          try {
            canonicalContentRef.current = nextContent;
            callbackRefs.onChange.current(nextContent);
          } catch (error) {
            fail(error);
          }
        },
      });
      lifecycleRef.current = lifecycle;
    } catch (error) {
      fail(error);
    }

    return () => {
      disposed = true;
      disposeEditorLifecycle(lifecycle);
      lifecycle = null;
      lifecycleRef.current = null;
    };
    // The editor is intentionally initialized once per mounted file surface.
    // Callback and document updates are synchronized by the effects below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
}

type ContentSyncOptions = Pick<
  LifecycleHookOptions,
  "lifecycleRef" | "canonicalContentRef" | "callbackRefs" | "content"
>;

function useHybridEditorContentSync({
  lifecycleRef,
  canonicalContentRef,
  callbackRefs,
  content,
}: ContentSyncOptions): void {
  useEffect(() => {
    const lifecycle = lifecycleRef.current;
    if (!lifecycle || canonicalContentRef.current === content) return;

    try {
      lifecycle.model.replaceSourceText(new StringValue(content));
      canonicalContentRef.current = content;
    } catch (error) {
      reportEditorFailure(lifecycleRef, callbackRefs, error);
    }
  }, [callbackRefs, canonicalContentRef, content, lifecycleRef]);
}

type OptionsSyncOptions = Pick<
  LifecycleHookOptions,
  "lifecycleRef" | "callbackRefs" | "readOnly" | "baseline" | "gutterMarkers" | "comments"
>;

function useHybridEditorOptionsSync({
  lifecycleRef,
  callbackRefs,
  readOnly,
  baseline,
  gutterMarkers,
  comments,
}: OptionsSyncOptions): void {
  useEffect(() => {
    const lifecycle = lifecycleRef.current;
    if (!lifecycle) return;

    try {
      setObservable(lifecycle.model.readonlyMode, readOnly);
      setObservable(
        lifecycle.model.baseline,
        baseline === undefined ? undefined : new StringValue(baseline),
      );
      setObservable(lifecycle.model.gutterMarkers, toGutterMarkers(gutterMarkers));
      lifecycle.commentsModel.set(toComments(comments));
    } catch (error) {
      reportEditorFailure(lifecycleRef, callbackRefs, error);
    }
  }, [baseline, callbackRefs, comments, gutterMarkers, lifecycleRef, readOnly]);
}

type CreateEditorLifecycleOptions = {
  root: HTMLDivElement;
  content: string;
  readOnly: boolean;
  baseline: string | undefined;
  gutterMarkers: readonly MarkdownGutterMarker[];
  comments: readonly MarkdownComment[];
  onOpenLink: NonNullable<EditorViewOptions["onOpenLink"]>;
  onComment: (submission: MarkdownCommentSubmission) => void;
  onSourceEdit: (content: string) => void;
};

function createEditorLifecycle({
  root,
  content,
  readOnly,
  baseline,
  gutterMarkers,
  comments,
  onOpenLink,
  onComment,
  onSourceEdit,
}: CreateEditorLifecycleOptions): EditorLifecycle {
  let model: EditorModel | undefined;
  let view: EditorView | undefined;
  let controller: EditorController | undefined;
  let commentsModel: CommentsModel | undefined;
  let commentsView: CommentsView | undefined;
  let commentMode: CommentModeController | undefined;
  let historyStrategy: LocalHistoryStrategy | undefined;
  let syntaxHighlighter: MonacoSyntaxHighlighter | undefined;
  let sourceEditSubscription: { dispose: () => void } | undefined;

  try {
    model = new EditorModel();
    setObservable(model.sourceText, new StringValue(content));
    setObservable(model.readonlyMode, readOnly);
    setObservable(model.baseline, baseline === undefined ? undefined : new StringValue(baseline));
    setObservable(model.gutterMarkers, toGutterMarkers(gutterMarkers));
    syntaxHighlighter = createMarkdownSyntaxHighlighter();

    const viewOptions: EditorViewOptions = {
      classNames: ["kandev-hybrid-markdown-editor"],
      showReadonlyToggle: false,
      onOpenLink,
      syntaxHighlighter,
    };
    view = new EditorView(model, viewOptions);
    root.replaceChildren(view.element);
    historyStrategy = new LocalHistoryStrategy(model);
    controller = new EditorController(model, view, {
      historyStrategy,
    });
    commentsModel = new CommentsModel();
    commentsModel.set(toComments(comments));
    commentsView = new CommentsView(commentsModel, view);
    commentMode = new CommentModeController(model, view, {
      onSubmit: (submission) => onComment(toCommentSubmission(submission)),
    });
    sourceEditSubscription = subscribeToAppliedSourceEdits(model, onSourceEdit);

    return {
      model,
      view,
      controller,
      commentsModel,
      commentsView,
      commentMode,
      historyStrategy,
      syntaxHighlighter,
      sourceEditSubscription,
    };
  } catch (error) {
    disposeEditorParts({
      model,
      view,
      controller,
      commentsModel,
      commentsView,
      commentMode,
      historyStrategy,
      syntaxHighlighter,
      sourceEditSubscription,
    });
    throw error;
  }
}

function reportEditorFailure(
  lifecycleRef: MutableRef<EditorLifecycle | null>,
  callbackRefs: EditorCallbackRefs,
  error: unknown,
): void {
  const lifecycle = lifecycleRef.current;
  if (lifecycle) disposeEditorLifecycle(lifecycle);
  lifecycleRef.current = null;
  callbackRefs.onError.current?.(error);
  callbackRefs.onSourceFallback.current?.();
}

function subscribeToAppliedSourceEdits(
  model: EditorModel,
  onSourceEdit: (content: string) => void,
): { dispose: () => void } {
  let active = true;
  let notificationScheduled = false;
  const subscription = model.onWillApplySourceEdit(() => {
    if (notificationScheduled) return;
    notificationScheduled = true;
    queueMicrotask(() => {
      notificationScheduled = false;
      if (!active) return;
      onSourceEdit(model.sourceText.get().value);
    });
  });

  return {
    dispose: () => {
      active = false;
      subscription.dispose();
    },
  };
}

function setObservable<T>(
  observable: { set: (value: T, transaction: undefined, change: undefined) => void },
  value: T,
): void {
  observable.set(value, undefined, undefined);
}

function toGutterMarkers(markers: readonly MarkdownGutterMarker[]) {
  return markers.map(({ start, endExclusive, kind }) => ({
    type: kind,
    range: new OffsetRange(start, endExclusive),
  }));
}

function toComments(comments: readonly MarkdownComment[]): Comment[] {
  return comments.map(({ id, start, endExclusive, body, author, createdAt }) => ({
    id,
    range: new OffsetRange(start, endExclusive),
    body,
    author,
    createdAt,
  }));
}

function toCommentSubmission(submission: CommentSubmission): MarkdownCommentSubmission {
  return {
    text: submission.text,
    start: submission.range.start,
    endExclusive: submission.range.endExclusive,
  };
}

function disposeEditorLifecycle(lifecycle: EditorLifecycle | null): void {
  if (!lifecycle) return;
  disposeEditorParts(lifecycle);
}

function disposeEditorParts(parts: Partial<EditorLifecycle>): void {
  parts.sourceEditSubscription?.dispose();
  parts.commentMode?.dispose();
  parts.commentsView?.dispose();
  parts.controller?.dispose();
  parts.view?.dispose();
  parts.syntaxHighlighter?.dispose();
  // The pinned package exposes EditorModel.dispose() at runtime, but its public
  // declaration does not include the lifecycle method.
  (parts.model as unknown as { dispose?: () => void } | undefined)?.dispose?.();
}

function createMarkdownSyntaxHighlighter(): MonacoSyntaxHighlighter {
  return createDefaultMonacoSyntaxHighlighter(
    { compile, MonarchTokenizer } as unknown as IMonarchApi,
    {
      typescript: typescriptLanguage,
      javascript: javascriptLanguage,
      css: cssLanguage,
      html: htmlLanguage,
      python: pythonLanguage,
      rust: rustLanguage,
      shell: shellLanguage,
      yaml: yamlLanguage,
    },
  );
}

function applyActiveTableEdit(
  lifecycleRef: MutableRef<EditorLifecycle | null>,
  operation: "row" | "column",
  index: number,
): void {
  const lifecycle = lifecycleRef.current;
  if (!lifecycle) return;
  const activeBlock = lifecycle.model.activeBlock.get();
  if (!activeBlock || activeBlock.kind !== "table") return;

  const range = findActiveTableRange(lifecycle.model, activeBlock);
  if (!range) return;
  const source = lifecycle.model.sourceText.get().value;
  const tableSource = source.slice(range.start, range.endExclusive);
  const result =
    operation === "row"
      ? insertMarkdownTableRow(tableSource, index)
      : insertMarkdownTableColumn(tableSource, index);
  if (result.source === tableSource) return;

  const edit = StringEdit.replace(new OffsetRange(range.start, range.endExclusive), result.source);
  lifecycle.historyStrategy.record(
    () =>
      lifecycle.model.applyEdit(edit, Selection.collapsed(range.start + result.selectionOffset)),
    edit,
  );
}

function findActiveTableRange(
  model: EditorModel,
  activeTable: TableAstNode,
): MarkdownSourceRange | undefined {
  let start = 0;
  for (const node of model.document.get().content) {
    if (node === activeTable) {
      return { start, endExclusive: start + node.length };
    }
    start += node.length;
  }
  return undefined;
}
