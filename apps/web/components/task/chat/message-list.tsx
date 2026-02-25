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

/** Change to "virtuoso" to switch back to react-virtuoso rendering. */
const STRATEGY = "native";

export const MessageList = memo(function MessageList(props: MessageListProps) {
  const Renderer = strategies[STRATEGY] ?? NativeMessageList;
  return <Renderer {...props} />;
});
