"use client";

import {
  createContext,
  useContext,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useChatMotion } from "@/hooks/use-chat-motion";
import { getNewArrivalIds, type ArrivalSnapshot } from "./chat-motion-state";

const ChatMotionContext = createContext({
  live: false,
  arrivals: new Set<string>() as ReadonlySet<string>,
  receivedAt: 0,
});

export function ChatMotionProvider({
  children,
  sessionId,
  messages,
  live: eligible,
}: {
  children: ReactNode;
  sessionId: string | null;
  messages: readonly { id: string }[];
  live: boolean;
}) {
  const enabled = useChatMotion();
  const live = eligible && enabled;
  const identity = messages.map((message) => message.id).join("\0");
  const ids = useMemo(() => (identity ? identity.split("\0") : []), [identity]);
  const previous = useRef<ArrivalSnapshot | null>(null);
  const snapshot = useMemo(() => ({ sessionId, ids, live }), [sessionId, ids, live]);
  const value = useMemo(
    () => ({
      live,
      arrivals: getNewArrivalIds(previous.current, snapshot),
      receivedAt: Date.now(),
    }),
    [live, snapshot],
  );
  useLayoutEffect(() => {
    previous.current = snapshot;
  }, [snapshot]);
  return <ChatMotionContext.Provider value={value}>{children}</ChatMotionContext.Provider>;
}

export function useChatTextMotion(): boolean {
  return useContext(ChatMotionContext).live;
}

/** The inner wrapper leaves measured row geometry and message identity intact. */
export function ChatMotionItem({
  children,
  messageId,
}: {
  children: ReactNode;
  messageId: string;
}) {
  const context = useContext(ChatMotionContext);
  const [arrival] = useState(() => (context.arrivals.has(messageId) ? context.receivedAt : null));
  const played = useRef(false);
  const ref = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    if (!context.live || arrival === null || played.current) return;
    played.current = true;
    const remaining = 160 - (Date.now() - arrival);
    if (remaining <= 0 || !ref.current?.animate) return;
    const animation = ref.current.animate(
      [
        { opacity: 0, transform: "translateY(3px)" },
        { opacity: 1, transform: "translateY(0)" },
      ],
      { duration: remaining, easing: "ease-out" },
    );
    return () => animation.cancel();
  }, [arrival, context.live]);
  return (
    <div ref={ref} data-chat-motion-item={messageId}>
      {children}
    </div>
  );
}
