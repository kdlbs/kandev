import type { StoreApi } from 'zustand';
import type { AppState, PendingPermission } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';
import type { PermissionRequestedPayload } from '@/lib/types/backend';

export function registerPermissionsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'permission.requested': (message) => {
      const payload = message.payload as PermissionRequestedPayload;
      const state = store.getState();
      // Convert to PendingPermission type (they're compatible)
      const permission: PendingPermission = {
        pending_id: payload.pending_id,
        task_id: payload.task_id,
        agent_instance_id: payload.agent_instance_id,
        session_id: payload.session_id,
        tool_call_id: payload.tool_call_id,
        title: payload.title,
        options: payload.options,
        action_type: payload.action_type,
        action_details: payload.action_details,
        created_at: payload.created_at,
        timestamp: payload.timestamp,
      };
      state.addPendingPermission(permission);
    },
  };
}

