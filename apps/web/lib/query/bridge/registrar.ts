import type { QueryClient } from "@tanstack/react-query";
import type { BackendMessageMap, BackendMessageType } from "@/lib/types/backend";
import type { WebSocketClient } from "@/lib/ws/client";
import { wrapBridgeHandler } from "./audit";

export type QueryBridgeCleanup = () => void;

export interface QueryBridgeRegistration {
  actions: readonly string[];
  cleanup: QueryBridgeCleanup;
}

export type QueryBridgeRegistrar = (
  ws: WebSocketClient,
  queryClient: QueryClient,
) => QueryBridgeRegistration;

type BridgeHandlerMap = {
  [Action in BackendMessageType]?: (message: BackendMessageMap[Action]) => void;
};

export function registerBridgeHandlers(
  ws: WebSocketClient,
  queryClient: QueryClient,
  handlers: BridgeHandlerMap,
): QueryBridgeRegistration {
  const cleanups = Object.entries(handlers).map(([action, handler]) =>
    ws.on(
      action as BackendMessageType,
      wrapBridgeHandler(
        queryClient,
        action,
        handler as (message: BackendMessageMap[BackendMessageType]) => void,
      ),
    ),
  );

  return {
    actions: Object.keys(handlers),
    cleanup: () => {
      cleanups.forEach((cleanup) => cleanup());
    },
  };
}
