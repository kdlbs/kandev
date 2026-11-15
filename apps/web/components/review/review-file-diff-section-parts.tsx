"use client";

import {
  useCallback,
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  type Dispatch,
  type RefObject,
  type SetStateAction,
} from "react";
import { ReviewDiffHeader, type ReviewExternalLinkContext } from "./review-diff-header";
import { useReviewHtmlPreview } from "./use-review-html-preview";
import type { ReviewFile } from "./types";

export function useLazyVisible(scrollContainer: RefObject<HTMLDivElement | null>) {
  const [isVisible, setIsVisible] = useState(false);
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel) return;
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setIsVisible(true);
          observer.disconnect();
        }
      },
      { rootMargin: "200px 0px", root: scrollContainer.current },
    );
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [scrollContainer]);
  return { isVisible, sentinelRef };
}

type AutoMarkArgs = {
  autoMarkOnScroll: boolean;
  isReviewed: boolean;
  isStale: boolean;
  /** Composite per-file key (matches what onToggleReviewed expects). */
  fileKey: string;
  onToggleReviewed: (key: string, reviewed: boolean) => void;
  scrollContainer: RefObject<HTMLDivElement | null>;
  suppressAutoMark: RefObject<boolean>;
};

export function useAutoMarkOnScroll({
  autoMarkOnScroll,
  isReviewed,
  isStale,
  fileKey,
  onToggleReviewed,
  scrollContainer,
  suppressAutoMark,
}: AutoMarkArgs) {
  const scrollSentinelRef = useRef<HTMLDivElement | null>(null);
  const markIfEligible = useEffectEvent(() => {
    if (!isReviewed && !isStale && !suppressAutoMark.current) {
      onToggleReviewed(fileKey, true);
    }
  });
  useEffect(() => {
    if (!autoMarkOnScroll) return;
    const sentinel = scrollSentinelRef.current;
    const root = scrollContainer.current;
    if (!sentinel || !root) return;
    let active = true;
    let previousAbove: boolean | null = null;
    let previousOffset: number | null = null;
    let previousScrollTop = root.scrollTop;
    const updatePosition = (allowMark: boolean) => {
      if (!active) return;
      const offset = sentinel.getBoundingClientRect().top - root.getBoundingClientRect().top;
      const above = offset < 0;
      const scrollDelta = root.scrollTop - previousScrollTop;
      // A scroll pass moves the marker by the scroll delta; layout shifts change that offset.
      const movedWithScroll =
        previousOffset !== null && Math.abs(previousOffset - offset - scrollDelta) < 1;
      const crossed = previousAbove === false && above && scrollDelta > 0 && movedWithScroll;
      previousAbove = above;
      previousOffset = offset;
      previousScrollTop = root.scrollTop;
      if (allowMark && crossed) markIfEligible();
    };
    const handleScroll = () => updatePosition(true);
    // Layout observations refresh the baseline without certifying a reading pass.
    const observer = new IntersectionObserver(() => updatePosition(false), { threshold: 0, root });
    observer.observe(sentinel);
    root.addEventListener("scroll", handleScroll, { passive: true });
    return () => {
      active = false;
      root.removeEventListener("scroll", handleScroll);
      observer.disconnect();
    };
  }, [autoMarkOnScroll, fileKey, scrollContainer]);
  return scrollSentinelRef;
}

export function useScrollIntoViewOnSelect(
  isSelected: boolean | undefined,
  sectionRef: RefObject<HTMLDivElement | null> | undefined,
  setCollapsed: Dispatch<SetStateAction<boolean>>,
  suppressAutoMark: RefObject<boolean>,
) {
  useEffect(() => {
    if (isSelected) {
      suppressAutoMark.current = true;
      setCollapsed(false);
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          sectionRef?.current?.scrollIntoView({ behavior: "smooth", block: "start" });
        });
      });
    }
  }, [isSelected, sectionRef, setCollapsed, suppressAutoMark]);
}

export function useFileDiffDisplayControls(wordWrap: boolean) {
  const [collapsed, setCollapsed] = useState(false);
  const [expandUnchanged, setExpandUnchanged] = useState(false);
  const [localWordWrap, setLocalWordWrap] = useState<boolean | undefined>(undefined);
  const effectiveWordWrap = localWordWrap ?? wordWrap;
  const handleToggleCollapse = useCallback(() => setCollapsed((v) => !v), []);
  const handleToggleExpandUnchanged = useCallback(() => setExpandUnchanged((v) => !v), []);
  const handleToggleWordWrap = useCallback(
    () => setLocalWordWrap((v) => !(v ?? wordWrap)),
    [wordWrap],
  );
  return {
    collapsed,
    setCollapsed,
    expandUnchanged,
    effectiveWordWrap,
    handleToggleCollapse,
    handleToggleExpandUnchanged,
    handleToggleWordWrap,
  };
}

type FileDiffVisibilityArgs = {
  scrollContainer: RefObject<HTMLDivElement | null>;
  sectionRef?: RefObject<HTMLDivElement | null>;
  isSelected?: boolean;
  setCollapsed: Dispatch<SetStateAction<boolean>>;
  suppressAutoMark: RefObject<boolean>;
  autoMarkOnScroll: boolean;
  isReviewed: boolean;
  isStale: boolean;
  fileKey: string;
  file: ReviewFile;
  onToggleReviewed: (key: string, reviewed: boolean) => void;
};

export function useFileDiffVisibility(args: FileDiffVisibilityArgs) {
  const { isVisible, sentinelRef } = useLazyVisible(args.scrollContainer);
  useScrollIntoViewOnSelect(
    args.isSelected,
    args.sectionRef,
    args.setCollapsed,
    args.suppressAutoMark,
  );
  const scrollSentinelRef = useAutoMarkOnScroll({
    autoMarkOnScroll:
      args.autoMarkOnScroll &&
      args.file.diff_state !== "pending" &&
      args.file.diff_state !== "unavailable",
    isReviewed: args.isReviewed,
    isStale: args.isStale,
    fileKey: args.fileKey,
    onToggleReviewed: args.onToggleReviewed,
    scrollContainer: args.scrollContainer,
    suppressAutoMark: args.suppressAutoMark,
  });
  return { isVisible, sentinelRef, scrollSentinelRef };
}

/** Row-local preview state plus the shared header props that toggle it. */
export function useFileDiffHtmlPreview(
  file: ReviewFile,
  sessionId: string,
  taskId: string | null | undefined,
  setCollapsed: (value: boolean) => void,
  suppressAutoMark: RefObject<boolean>,
) {
  const preview = useReviewHtmlPreview({ file, sessionId, taskId, suppressAutoMark });
  const toggle = useCallback(() => {
    if (!preview.isActive) setCollapsed(false);
    preview.toggle();
  }, [preview.isActive, preview.toggle, setCollapsed]);
  return {
    previewProps: preview.previewProps,
    headerProps: {
      htmlPreview: preview.isActive,
      onToggleHtmlPreview: preview.canPreview ? toggle : undefined,
    },
  };
}

type FileDiffSectionHeaderProps = {
  file: ReviewFile;
  isReviewed: boolean;
  isStale: boolean;
  sessionId: string;
  controls: ReturnType<typeof useFileDiffDisplayControls>;
  hasStickyRepoHeader: boolean;
  onCheckboxChange: (checked: boolean | "indeterminate") => void;
  onDiscard: () => void;
  onCommentFile: () => void;
  onOpenFile?: (filePath: string, repo?: string) => void;
  markdownPreview?: boolean;
  onToggleMarkdownPreview?: () => void;
  htmlPreview: boolean;
  onToggleHtmlPreview?: () => void;
  externalLinkContext: ReviewExternalLinkContext;
};

export function FileDiffSectionHeader({
  file,
  isReviewed,
  isStale,
  sessionId,
  controls,
  hasStickyRepoHeader,
  onCheckboxChange,
  onDiscard,
  onCommentFile,
  onOpenFile,
  markdownPreview,
  onToggleMarkdownPreview,
  htmlPreview,
  onToggleHtmlPreview,
  externalLinkContext,
}: FileDiffSectionHeaderProps) {
  return (
    <ReviewDiffHeader
      file={file}
      isReviewed={isReviewed}
      isStale={isStale}
      sessionId={sessionId}
      collapsed={controls.collapsed}
      wordWrap={controls.effectiveWordWrap}
      expandUnchanged={controls.expandUnchanged}
      hasStickyRepoHeader={hasStickyRepoHeader}
      onCheckboxChange={onCheckboxChange}
      onDiscard={onDiscard}
      onCommentFile={onCommentFile}
      onOpenFile={onOpenFile}
      markdownPreview={markdownPreview}
      onToggleMarkdownPreview={onToggleMarkdownPreview}
      htmlPreview={htmlPreview}
      onToggleHtmlPreview={onToggleHtmlPreview}
      onToggleCollapse={controls.handleToggleCollapse}
      onToggleExpandUnchanged={controls.handleToggleExpandUnchanged}
      onToggleWordWrap={controls.handleToggleWordWrap}
      {...externalLinkContext}
    />
  );
}
