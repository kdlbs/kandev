"use client";

import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ComponentProps } from "react";
import type { ExtraProps } from "react-markdown";
import {
  appendedTextRuns,
  CHAT_TEXT_DURATION_MS,
  splitMotionText,
  type TextRun,
} from "@/lib/markdown/chat-text-motion";
import { chatTextMotionPlugin } from "@/lib/markdown/chat-text-motion-plugin";

export function ChatMotionSpan({
  node,
  ...props
}: ComponentProps<"span"> & ExtraProps & { "data-chat-text-motion"?: number }) {
  const ref = useRef<HTMLSpanElement>(null);
  const receivedAt = Number(
    node?.properties?.["data-chat-text-motion"] ?? props["data-chat-text-motion"],
  );
  useLayoutEffect(() => {
    const elapsed = Math.max(0, Date.now() - receivedAt);
    if (!Number.isFinite(receivedAt) || elapsed >= CHAT_TEXT_DURATION_MS || !ref.current?.animate)
      return;
    const animation = ref.current.animate(
      [{ opacity: 0.6 + (0.4 * elapsed) / CHAT_TEXT_DURATION_MS }, { opacity: 1 }],
      { duration: CHAT_TEXT_DURATION_MS - elapsed, easing: "linear" },
    );
    return () => animation.cancel();
  }, [receivedAt]);
  return <span {...props} ref={ref} />;
}

function hasTextSelection(): boolean {
  return typeof window !== "undefined" && window.getSelection()?.isCollapsed === false;
}

function useChatTextRuns(content: string, enabled: boolean) {
  const [snapshot, setSnapshot] = useState({ content, enabled, runs: [] as TextRun[] });
  let current = snapshot;
  if (snapshot.content !== content || snapshot.enabled !== enabled) {
    let runs: TextRun[] = [];
    if (enabled && snapshot.enabled) {
      runs = hasTextSelection()
        ? snapshot.runs
        : appendedTextRuns(snapshot.content, content, snapshot.runs, Date.now());
    }
    current = { content, enabled, runs };
    setSnapshot(current);
  }
  const runs = current.runs;
  useEffect(() => {
    const last = runs.at(-1);
    if (!last) return;
    const compact = () => {
      if (hasTextSelection()) return;
      document.removeEventListener("selectionchange", compact);
      setSnapshot((value) => ({ ...value, runs: [] }));
    };
    const timer = setTimeout(
      () => {
        if (hasTextSelection()) document.addEventListener("selectionchange", compact);
        else compact();
      },
      Math.max(0, last.receivedAt + CHAT_TEXT_DURATION_MS - Date.now()),
    );
    return () => {
      clearTimeout(timer);
      document.removeEventListener("selectionchange", compact);
    };
  }, [runs]);
  return runs;
}

export function useChatMarkdownMotion(content: string, enabled: boolean) {
  const runs = useChatTextRuns(content, enabled);
  return useMemo(() => (runs.length ? [chatTextMotionPlugin(content, runs)] : []), [content, runs]);
}

export function ChatAnimatedText({ text, enabled }: { text: string; enabled: boolean }) {
  const runs = useChatTextRuns(text, enabled);
  return splitMotionText(text, 0, text, runs).map((part, index) =>
    part.receivedAt === undefined ? (
      part.text
    ) : (
      <ChatMotionSpan key={index} data-chat-text-motion={part.receivedAt}>
        {part.text}
      </ChatMotionSpan>
    ),
  );
}
