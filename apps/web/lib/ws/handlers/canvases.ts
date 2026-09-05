import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { recordCanvasLifecycle, type CanvasLifecycleAction } from "@/lib/canvas-lifecycle";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { CanvasLifecyclePayload } from "@/lib/types/backend";
import type { BackendMessage } from "@/lib/types/backend-message";

/**
 * Canvas lifecycle notifications carry identity and status only. Visible
 * task, workspace, and direct-host projections refetch their own scoped HTTP
 * snapshot so no WebSocket payload becomes a second canvas source of truth.
 */
export function registerCanvasesHandlers(_store: StoreApi<AppState>): WsHandlers {
  const invalidate = (message: BackendMessage<CanvasLifecycleAction, CanvasLifecyclePayload>) =>
    recordCanvasLifecycle(message.action, message.payload);
  return {
    "canvas.created": invalidate,
    "canvas.release.activated": invalidate,
    "canvas.release.permission_required": invalidate,
    "canvas.promoted": invalidate,
    "canvas.archived": invalidate,
    "canvas.restored": invalidate,
    "canvas.removed": invalidate,
  };
}
