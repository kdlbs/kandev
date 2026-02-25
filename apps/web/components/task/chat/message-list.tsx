"use client";

import type { ComponentType } from "react";
import { memo } from "react";
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
 */
const STRATEGY = "native";

export const MessageList = memo(function MessageList(props: MessageListProps) {
  const Renderer = strategies[STRATEGY] ?? NativeMessageList;
  return <Renderer {...props} />;
});
