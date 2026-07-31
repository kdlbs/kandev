"use client";

import {
  memo,
  useMemo,
  forwardRef,
  type ForwardRefExoticComponent,
  type RefAttributes,
} from "react";
import { NativeMessageList } from "./message-list-native";
import { VirtuosoMessageList } from "./message-list-virtuoso";
import type { MessageListProps, MessageListHandle } from "./message-list-shared";

type Strategy = ForwardRefExoticComponent<MessageListProps & RefAttributes<MessageListHandle>>;

const strategies: Record<string, Strategy> = {
  native: NativeMessageList,
  virtuoso: VirtuosoMessageList,
};

/**
 * Rendering strategy for the message list.
 * - "native": simple DOM rendering with overflow-anchor for scroll pinning.
 *   Better for short/medium conversations; avoids Virtuoso measurement quirks.
 * - "virtuoso": react-virtuoso windowed rendering.
 *   Better for very long conversations (1000+ messages) where DOM node count matters.
 *
 * Overridable at runtime via ?renderer=virtuoso|native query param (used by E2E
 * coverage to exercise both paths without redeploying).
 */
const STRATEGY = "native";

function resolveStrategy(): string {
  if (typeof window === "undefined") return STRATEGY;
  const override = new URLSearchParams(window.location.search).get("renderer");
  return override && override in strategies ? override : STRATEGY;
}

export const MessageList = memo(
  forwardRef<MessageListHandle, MessageListProps>(function MessageList(props, ref) {
    const key = useMemo(() => resolveStrategy(), []);
    const Renderer = strategies[key] ?? NativeMessageList;
    return <Renderer {...props} ref={ref} />;
  }),
);
