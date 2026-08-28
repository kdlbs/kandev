"use client";

import { useEffect, useRef } from "react";
import {
  CommentModeController,
  CommentsModel,
  CommentsView,
  EditorController,
  EditorModel,
  EditorView,
  LocalHistoryStrategy,
  OffsetRange,
  StringValue,
  type Comment,
  type CommentSubmission,
  type EditorViewOptions,
} from "@vscode/markdown-editor";
import "@vscode/markdown-editor/editor.css";
import "@vscode/markdown-editor/themes/default.css";
import {
  applyMarkdownSourceEdit,
  type MarkdownSourceReplacement,
} from "./markdown-source-preservation";

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
  onOpenLink?: (url: string) => void;
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
  sourceEditSubscription: { dispose: () => void };
};

type SourceEditLike = {
  replacements: readonly MarkdownSourceReplacementLike[];
};

type MarkdownSourceReplacementLike = {
  replaceRange: MarkdownSourceRange;
  newText: string;
};

type EditorCallbackRefs = {
  onChange: { current: (content: string) => void };
  onOpenLink: { current: ((url: string) => void) | undefined };
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

  return <div ref={rootRef} className={className} data-testid="hybrid-markdown-editor" />;
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
          callbackRefs.onOpenLink.current?.(url);
          return callbackRefs.onOpenLink.current ? undefined : false;
        },
        onComment: (submission) => callbackRefs.onComment.current?.(submission),
        onSourceEdit: (edit) => {
          try {
            const nextContent = applyMarkdownSourceEdit(
              canonicalContentRef.current,
              toSourceReplacements(edit),
            );
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
  onSourceEdit: (edit: SourceEditLike) => void;
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
  let sourceEditSubscription: { dispose: () => void } | undefined;

  try {
    model = new EditorModel();
    setObservable(model.sourceText, new StringValue(content));
    setObservable(model.readonlyMode, readOnly);
    setObservable(model.baseline, baseline === undefined ? undefined : new StringValue(baseline));
    setObservable(model.gutterMarkers, toGutterMarkers(gutterMarkers));

    const viewOptions: EditorViewOptions = {
      classNames: ["md-theme-default", "kandev-hybrid-markdown-editor"],
      showReadonlyToggle: false,
      onOpenLink,
    };
    view = new EditorView(model, viewOptions);
    root.replaceChildren(view.element);
    controller = new EditorController(model, view, {
      historyStrategy: new LocalHistoryStrategy(model),
    });
    commentsModel = new CommentsModel();
    commentsModel.set(toComments(comments));
    commentsView = new CommentsView(commentsModel, view);
    commentMode = new CommentModeController(model, view, {
      onSubmit: (submission) => onComment(toCommentSubmission(submission)),
    });
    sourceEditSubscription = model.onWillApplySourceEdit((event) => onSourceEdit(event.edit));

    return {
      model,
      view,
      controller,
      commentsModel,
      commentsView,
      commentMode,
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

function toSourceReplacements(edit: SourceEditLike): MarkdownSourceReplacement[] {
  return edit.replacements.map(({ replaceRange, newText }) => ({
    start: replaceRange.start,
    endExclusive: replaceRange.endExclusive,
    newText,
  }));
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
  // The pinned package exposes EditorModel.dispose() at runtime, but its public
  // declaration does not include the lifecycle method.
  (parts.model as unknown as { dispose?: () => void } | undefined)?.dispose?.();
}
