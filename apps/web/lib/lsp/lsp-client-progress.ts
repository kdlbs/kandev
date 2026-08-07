import type { JsonRpcConnection } from "./lsp-json-rpc";
import type { ManagedLspConnection } from "./lsp-client-types";
import { applyLspProgress, createLspProgressSnapshot, isLspProgressToken } from "./lsp-progress";

export function beginLspProgressTracking(
  connection: ManagedLspConnection,
  rpc: JsonRpcConnection,
  isCurrent: () => boolean,
  notifyChange: () => void,
): void {
  connection.progress = createLspProgressSnapshot(Date.now());
  connection.registeredProgressTokens.add(connection.ownerId);
  notifyChange();

  rpc.onRequest("window/workDoneProgress/create", (params: unknown) => {
    if (!isCurrent() || typeof params !== "object" || params === null) return null;
    const token = (params as { token?: unknown }).token;
    if (isLspProgressToken(token)) connection.registeredProgressTokens.add(token);
    return null;
  });

  rpc.onNotification("$/progress", (params: unknown) => {
    if (!isCurrent()) return;
    const progress = applyLspProgress(
      connection.progress,
      connection.registeredProgressTokens,
      params,
      Date.now(),
    );
    if (progress === connection.progress) return;
    connection.progress = progress;
    notifyChange();
  });
}
