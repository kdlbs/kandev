import type { QueryClient } from "@tanstack/react-query";
import type { BackendMessageMap, BackendMessageType } from "@/lib/types/backend";
import type { WebSocketClient } from "@/lib/ws/client";
import { installBridgeAuditAccessors, recordBridgeAllowlistedEvent } from "./audit";
import { registerOfficeBridge } from "./office";
import { registerSessionBridge } from "./session";
import { registerSessionRuntimeBridge } from "./session-runtime";
import { registerSettingsSystemBridge } from "./settings-system";
import { registerTaskBridge } from "./tasks";
import { registerWorkspaceBridge } from "./workspace";
import type { QueryBridgeRegistrar } from "./registrar";

export {
  BRIDGE_SKIPPED_ACTIONS,
  BRIDGE_SKIPPED_PREFIXES,
  clearBridgeAuditRows,
  getBridgeAuditRows,
  getBridgeSkipReason,
  isBridgeAuditEnabled,
  isBridgeSkippedAction,
  wrapBridgeHandler,
  type BridgeAuditEntry,
  type BridgeAuditStatus,
} from "./audit";
export type {
  QueryBridgeCleanup,
  QueryBridgeRegistrar,
  QueryBridgeRegistration,
} from "./registrar";

export interface QueryBridgeOptions {
  registrars?: readonly QueryBridgeRegistrar[];
}

const DEFAULT_REGISTRARS: readonly QueryBridgeRegistrar[] = [
  registerTaskBridge,
  registerWorkspaceBridge,
  registerOfficeBridge,
  registerSessionBridge,
  registerSessionRuntimeBridge,
  registerSettingsSystemBridge,
];

export function registerQueryBridge(
  ws: WebSocketClient,
  queryClient: QueryClient,
  options: QueryBridgeOptions = {},
): () => void {
  installBridgeAuditAccessors();

  const registrations = (options.registrars ?? DEFAULT_REGISTRARS).map((registrar) =>
    registrar(ws, queryClient),
  );
  const handledActions = new Set(registrations.flatMap((registration) => registration.actions));
  const cleanups = registrations.map((registration) => registration.cleanup);

  cleanups.push(registerAllowlistAudit(ws, handledActions));

  return () => {
    cleanups.forEach((cleanup) => cleanup());
  };
}

function registerAllowlistAudit(ws: WebSocketClient, handledActions: ReadonlySet<string>) {
  return ws.onEnvelope((message: BackendMessageMap[BackendMessageType]) => {
    const auditMessage = message as BridgeAuditMessage;
    if (!auditMessage.action || handledActions.has(auditMessage.action)) return;
    recordBridgeAllowlistedEvent(auditMessage);
  });
}

type BridgeAuditMessage = {
  action?: string;
  payload?: unknown;
  type?: string;
};
