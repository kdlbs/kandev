import { useCallback, useLayoutEffect, useRef } from "react";
import { djb2Hash } from "@/lib/utils/hash";
import {
  captureContentLineAnchor,
  resolveContentLineAnchor,
  type ContentLineAnchor,
} from "./line-content-anchor";
import { reviewFileKey, type ReviewFile } from "./types";

type ScrollRootRef = React.RefObject<HTMLDivElement | null>;

export type ReviewScrollAnchor = {
  fileKey: string;
  line: number;
  side: string;
  lineText?: string;
  beforeLines?: string[];
  afterLines?: string[];
  offset: number;
  scrollTop: number;
  scrollLeft: number;
};

function lineSide(line: HTMLElement): string {
  const type = line.dataset.lineType ?? "context";
  if (type.includes("addition") || type.includes("add")) return "addition";
  if (type.includes("deletion") || type.includes("delete")) return "deletion";
  return "context";
}

function linesInFile(fileSection: HTMLElement): HTMLElement[] {
  const diff = fileSection.querySelector("diffs-container");
  const shadowRoot = diff?.shadowRoot;
  if (!shadowRoot) return [];
  return Array.from(shadowRoot.querySelectorAll<HTMLElement>("[data-line][data-line-type]"));
}

export function captureReviewScrollAnchor(root: HTMLElement): ReviewScrollAnchor | null {
  const rootRect = root.getBoundingClientRect();
  const sections = Array.from(root.querySelectorAll<HTMLElement>("[data-review-file-key]"));
  let first: { section: HTMLElement; line: HTMLElement; fileKey: string; top: number } | null =
    null;

  for (const section of sections) {
    const fileKey = section.dataset.reviewFileKey;
    if (!fileKey) continue;
    for (const line of linesInFile(section)) {
      const rect = line.getBoundingClientRect();
      if (rect.bottom <= rootRect.top || rect.top >= rootRect.bottom) continue;
      if (!first || rect.top < first.top) first = { section, line, fileKey, top: rect.top };
    }
  }

  if (!first) {
    const firstSection = sections
      .map((section) => ({ section, rect: section.getBoundingClientRect() }))
      .filter(({ rect }) => rect.bottom > rootRect.top && rect.top < rootRect.bottom)
      .sort((left, right) => left.rect.top - right.rect.top)[0];
    if (!firstSection?.section.dataset.reviewFileKey) return null;
    return {
      fileKey: firstSection.section.dataset.reviewFileKey,
      line: -1,
      side: "section",
      offset: firstSection.rect.top - rootRect.top,
      scrollTop: root.scrollTop,
      scrollLeft: root.scrollLeft,
    };
  }
  const visibleAnchor = first;
  const line = Number(visibleAnchor.line.dataset.line);
  if (!Number.isFinite(line)) return null;
  const side = lineSide(visibleAnchor.line);
  const sameSide = linesInFile(visibleAnchor.section)
    .filter(
      (candidate) =>
        lineSide(candidate) === side && Number.isFinite(Number(candidate.dataset.line)),
    )
    .map((candidate) => ({
      element: candidate,
      line: Number(candidate.dataset.line),
      content: candidate.textContent ?? "",
    }));
  const anchorContent = captureContentLineAnchor(
    sameSide,
    sameSide.findIndex((candidate) => candidate.element === visibleAnchor.line),
  );
  return {
    fileKey: visibleAnchor.fileKey,
    line,
    side,
    lineText: anchorContent?.content,
    beforeLines: anchorContent?.beforeLines,
    afterLines: anchorContent?.afterLines,
    offset: visibleAnchor.top - rootRect.top,
    scrollTop: root.scrollTop,
    scrollLeft: root.scrollLeft,
  };
}

function clamp(value: number, max: number): number {
  return Math.max(0, Math.min(value, max));
}

export function restoreReviewScrollAnchor(
  root: HTMLElement,
  anchor: ReviewScrollAnchor,
): "line" | "section" | "clamped" {
  const section = Array.from(root.querySelectorAll<HTMLElement>("[data-review-file-key]")).find(
    (candidate) => candidate.dataset.reviewFileKey === anchor.fileKey,
  );
  if (!section) {
    root.scrollTop = clamp(anchor.scrollTop, root.scrollHeight - root.clientHeight);
    root.scrollLeft = clamp(anchor.scrollLeft, root.scrollWidth - root.clientWidth);
    return "clamped";
  }

  if (anchor.side === "section") {
    const rootRect = root.getBoundingClientRect();
    const sectionTop = section.getBoundingClientRect().top;
    root.scrollTop = clamp(
      root.scrollTop + sectionTop - (rootRect.top + anchor.offset),
      root.scrollHeight - root.clientHeight,
    );
    root.scrollLeft = clamp(anchor.scrollLeft, root.scrollWidth - root.clientWidth);
    return "section";
  }

  const candidates = linesInFile(section).map((element) => ({
    element,
    line: Number(element.dataset.line),
    side: lineSide(element),
    content: element.textContent ?? "",
  }));
  const sameSide = candidates.filter(
    (candidate) => candidate.side === anchor.side && Number.isFinite(candidate.line),
  );
  const contentAnchor: ContentLineAnchor | null =
    anchor.lineText === undefined
      ? null
      : {
          line: anchor.line,
          content: anchor.lineText,
          beforeLines: anchor.beforeLines ?? [],
          afterLines: anchor.afterLines ?? [],
        };
  const mapped = contentAnchor ? resolveContentLineAnchor(sameSide, contentAnchor) : null;
  const exact = sameSide.find((candidate) => candidate.line === anchor.line);
  const nearest =
    mapped ??
    exact ??
    sameSide
      .filter((candidate) => Number.isFinite(candidate.line))
      .sort(
        (left, right) => Math.abs(left.line - anchor.line) - Math.abs(right.line - anchor.line),
      )[0];
  if (!nearest) {
    root.scrollTop = clamp(anchor.scrollTop, root.scrollHeight - root.clientHeight);
    root.scrollLeft = clamp(anchor.scrollLeft, root.scrollWidth - root.clientWidth);
    return "clamped";
  }

  const rootRect = root.getBoundingClientRect();
  const lineTop = nearest.element.getBoundingClientRect().top;
  root.scrollTop = clamp(
    root.scrollTop + lineTop - (rootRect.top + anchor.offset),
    root.scrollHeight - root.clientHeight,
  );
  root.scrollLeft = clamp(anchor.scrollLeft, root.scrollWidth - root.clientWidth);
  return "line";
}

function contentSignature(files: ReviewFile[]): string {
  return files
    .map((file) => {
      const identity = [
        reviewFileKey(file),
        file.source,
        file.repository_id ?? "",
        file.change_layer ?? "",
        file.display_scope_key ?? "",
        file.status,
        file.old_path ?? "",
        file.diff_skip_reason ?? "",
      ].join("\u0000");
      return `${identity}\u0000${file.diff.length}:${djb2Hash(file.diff)}`;
    })
    .sort()
    .join("\u0001");
}

function scopeSignature(files: ReviewFile[]): string {
  return Array.from(
    new Set(
      files.map((file) =>
        [
          file.source,
          file.repository_id ?? file.repository_name ?? "",
          file.change_layer ?? "",
          file.display_scope_key ?? file.base_ref ?? "",
        ].join("\u0000"),
      ),
    ),
  )
    .sort()
    .join("\u0001");
}

export function useReviewScrollAnchor({
  rootRef,
  files,
  sessionId,
  sourceKey,
  suppressAutoMark,
}: {
  rootRef: ScrollRootRef;
  files: ReviewFile[];
  sessionId: string;
  sourceKey?: string;
  suppressAutoMark: React.RefObject<boolean>;
}) {
  const anchorRef = useRef<ReviewScrollAnchor | null>(null);
  const frameRef = useRef<number | null>(null);
  const savedSuppressionRef = useRef<boolean | null>(null);
  const targetRef = useRef("");
  const contentRef = useRef("");
  const userInputRef = useRef(0);
  const currentContent = contentSignature(files);
  const currentTarget = `${sessionId}\u0000${sourceKey ?? ""}\u0000${scopeSignature(files)}`;

  const restoreSuppression = useCallback(() => {
    if (savedSuppressionRef.current === null) return;
    suppressAutoMark.current = savedSuppressionRef.current;
    savedSuppressionRef.current = null;
  }, [suppressAutoMark]);

  const cancelRestore = useCallback(() => {
    userInputRef.current += 1;
    if (frameRef.current !== null) cancelAnimationFrame(frameRef.current);
    frameRef.current = null;
    restoreSuppression();
  }, [restoreSuppression]);

  const captureOnScroll = useCallback(() => {
    if (frameRef.current !== null || contentRef.current !== currentContent) return;
    const root = rootRef.current;
    if (root) anchorRef.current = captureReviewScrollAnchor(root);
  }, [currentContent, rootRef]);

  const handleUserInput = useCallback(() => {
    cancelRestore();
    const root = rootRef.current;
    if (root) anchorRef.current = captureReviewScrollAnchor(root);
  }, [cancelRestore, rootRef]);

  useLayoutEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    if (targetRef.current !== currentTarget) {
      cancelRestore();
      targetRef.current = currentTarget;
      contentRef.current = currentContent;
      anchorRef.current = captureReviewScrollAnchor(root);
      return;
    }

    if (contentRef.current !== currentContent) {
      const anchor = anchorRef.current ?? captureReviewScrollAnchor(root);
      const savedSuppression = savedSuppressionRef.current ?? suppressAutoMark.current;
      const userInputVersion = userInputRef.current;
      contentRef.current = currentContent;
      if (anchor) {
        if (frameRef.current !== null) cancelAnimationFrame(frameRef.current);
        savedSuppressionRef.current = savedSuppression;
        suppressAutoMark.current = true;
        frameRef.current = requestAnimationFrame(() => {
          frameRef.current = null;
          if (userInputRef.current !== userInputVersion) {
            restoreSuppression();
            return;
          }
          restoreReviewScrollAnchor(root, anchor);
          restoreSuppression();
          anchorRef.current = captureReviewScrollAnchor(root) ?? anchor;
        });
      }
    }
  }, [cancelRestore, currentContent, currentTarget, restoreSuppression, rootRef, suppressAutoMark]);

  useLayoutEffect(() => cancelRestore, [cancelRestore]);

  return { captureOnScroll, handleUserInput };
}
