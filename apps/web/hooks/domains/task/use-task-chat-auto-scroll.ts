import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { selectCommandCount } from "@/lib/state/slices/session/selectors";
import type { TaskSession } from "@/components/task/simple/types";

const AUTOSCROLL_THRESHOLD_PX = 80;

function isAtBottom(scrollParent: HTMLElement | null): boolean {
  if (!scrollParent) return true; // window scroll case — be conservative.
  const remaining = scrollParent.scrollHeight - scrollParent.scrollTop - scrollParent.clientHeight;
  return remaining <= AUTOSCROLL_THRESHOLD_PX;
}

function scrollToBottom(scrollParent: HTMLElement | null): void {
  if (!scrollParent) return;
  scrollParent.scrollTop = scrollParent.scrollHeight;
}

/**
 * Auto-scroll the chat container to the bottom when new content arrives,
 * but only if the user was already near the bottom (within ~80px) at the
 * time of the change.
 *
 * Triggers on:
 *   - opening or switching conversations (unless linking to a comment)
 *   - comments arriving, including history loaded after mount
 *   - a new active session entry first appearing (active count grows)
 *   - new messages arriving in any session for this task
 *
 * Uses a scroll listener to track the user's "at-bottom" intent. Reads
 * the latest value before scrolling so we never yank focus from a user
 * who has scrolled up.
 */
export function useTaskChatAutoScroll(
  scrollParent: HTMLElement | null,
  sessions: TaskSession[],
  taskId: string,
  commentCount: number,
): void {
  const activeSessionCount = sessions.filter(
    (s) => s.state === "RUNNING" || s.state === "WAITING_FOR_INPUT",
  ).length;

  // Sum messages + command counts across all task sessions — single scalar
  // that grows whenever new content streams in.
  const totalContentSignal = useAppStore((s) => {
    let sum = 0;
    for (const session of sessions) {
      sum += s.messages.bySession[session.id]?.length ?? 0;
      sum += selectCommandCount(s, session.id);
    }
    return sum;
  });

  const wasAtBottomRef = useRef(true);

  useEffect(() => {
    if (!scrollParent) return;
    // A newly opened conversation follows the latest messages. Its initial
    // scrollTop is a browser default, not an intent to read older history.
    wasAtBottomRef.current = !window.location.hash.startsWith("#comment-");
    if (wasAtBottomRef.current) scrollToBottom(scrollParent);
    const handler = () => {
      if (scrollParent.clientHeight > 0) wasAtBottomRef.current = isAtBottom(scrollParent);
    };
    // Mobile tabs can mount the conversation before its container is visible.
    const observer =
      typeof ResizeObserver === "undefined"
        ? null
        : new ResizeObserver(() => {
            if (wasAtBottomRef.current && scrollParent.clientHeight > 0)
              scrollToBottom(scrollParent);
          });
    observer?.observe(scrollParent);
    scrollParent.addEventListener("scroll", handler, { passive: true });
    return () => {
      observer?.disconnect();
      scrollParent.removeEventListener("scroll", handler);
    };
  }, [scrollParent, taskId]);

  useEffect(() => {
    if (wasAtBottomRef.current) {
      scrollToBottom(scrollParent);
      // After programmatic scroll, we are still "at bottom" by definition.
      wasAtBottomRef.current = true;
    }
  }, [scrollParent, activeSessionCount, totalContentSignal, taskId, commentCount]);
}
