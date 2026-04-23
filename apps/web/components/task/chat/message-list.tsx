"use client";

import type { ComponentType } from "react";
import { memo, useMemo } from "react";
import { NativeMessageList } from "./message-list-native";
import { VirtuosoMessageList } from "./message-list-virtuoso";
import type { MessageListProps } from "./message-list-shared";

const strategies: Record<string, ComponentType<MessageListProps>> = {
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

export const MessageList = memo(function MessageList(props: MessageListProps) {
  const key = useMemo(resolveStrategy, []);
  const Renderer = strategies[key] ?? NativeMessageList;
  return <Renderer {...props} />;
});
