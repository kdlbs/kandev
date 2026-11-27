import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { emitAutomationRunRecorded } from "@/lib/automation-run-recorded";
import type { WsHandlers } from "./types";

export function registerAutomationRunHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "automation.run.created": ({ payload }) => {
      if (!payload?.workspace_id || payload.workspace_id !== store.getState().workspaces.activeId)
        return;
      emitAutomationRunRecorded(payload);
    },
  };
}
