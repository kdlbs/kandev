"use client";

import { useCallback, useState } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { Message } from "@/lib/types/http";
import type { PermissionActionType, PermissionOptionKind } from "@/lib/types/permission";

export type PermissionOption = {
  option_id: string;
  name: string;
  kind: PermissionOptionKind;
};

export type PermissionActionDetails = {
  command?: string;
  path?: string;
  cwd?: string;
  // Description forwarded from ToolCall.Title. Equals the displayed title
  // in the current backend; reserved for future use when agents send a
  // separate description distinct from Title.
  description?: string;
  // Raw tool-call input as sent by the agent (e.g. { command: "ls -la" },
  // { file_path: "foo.go", limit: 10 }, { url: "..." }). Schema varies per
  // tool; consumers should treat keys as opaque.
  raw_input?: Record<string, unknown>;
};

export type PermissionRequestMetadata = {
  pending_id: string;
  tool_call_id: string;
  options: PermissionOption[];
  action_type: PermissionActionType;
  action_details: PermissionActionDetails;
  status?: "pending" | "approved" | "rejected" | "expired";
};

export type ParsedPermission = {
  permissionMetadata: PermissionRequestMetadata | undefined;
  permissionStatus: PermissionRequestMetadata["status"];
  isPermissionPending: boolean;
};

export function parsePermission(permissionMessage: Message | undefined): ParsedPermission {
  const permissionMetadata = permissionMessage?.metadata as PermissionRequestMetadata | undefined;
  const permissionStatus = permissionMetadata?.status;
  const isPermissionPending =
    !!permissionMessage && (!permissionStatus || permissionStatus === "pending");
  return { permissionMetadata, permissionStatus, isPermissionPending };
}

type UsePermissionHandlersParams = {
  permissionMetadata: PermissionRequestMetadata | undefined;
  permissionMessage: Message | undefined;
};

export function usePermissionResponseHandlers({
  permissionMetadata,
  permissionMessage,
}: UsePermissionHandlersParams) {
  const [isResponding, setIsResponding] = useState(false);

  const handleRespond = useCallback(
    async (optionId: string, cancelled: boolean = false, rejected: boolean = false) => {
      if (!permissionMetadata || !permissionMessage) return;
      const client = getWebSocketClient();
      if (!client) {
        console.error("WebSocket client not available");
        return;
      }
      setIsResponding(true);
      try {
        await client.request("permission.respond", {
          session_id: permissionMessage.session_id,
          pending_id: permissionMetadata.pending_id,
          option_id: cancelled ? undefined : optionId,
          cancelled,
          rejected,
        });
      } catch (error) {
        console.error("Failed to respond to permission request:", error);
      } finally {
        setIsResponding(false);
      }
    },
    [permissionMessage, permissionMetadata],
  );

  // "Approve" is the one-shot allow. Prefer an explicit allow_once option and
  // only fall back to allow_always when the agent offers nothing else, so the
  // dedicated "Always allow" button (handleAllowAlways) stays distinct.
  const handleApprove = useCallback(() => {
    const options = permissionMetadata?.options ?? [];
    const allowOption =
      options.find((opt) => opt.kind === "allow_once") ??
      options.find((opt) => opt.kind === "allow_always");
    if (allowOption) handleRespond(allowOption.option_id);
  }, [permissionMetadata, handleRespond]);

  // "Always allow" maps to the agent's allow_always option, telling the agent
  // to persist the decision so the same action is not re-prompted. Only some
  // agents offer it (Cursor does); hasAllowAlways gates the button.
  const allowAlwaysOption = permissionMetadata?.options.find((opt) => opt.kind === "allow_always");
  const hasAllowAlways = !!allowAlwaysOption;
  const handleAllowAlways = useCallback(() => {
    if (allowAlwaysOption) handleRespond(allowAlwaysOption.option_id);
  }, [allowAlwaysOption, handleRespond]);

  const handleReject = useCallback(() => {
    const rejectOption = permissionMetadata?.options.find(
      (opt) => opt.kind === "reject_once" || opt.kind === "reject_always",
    );
    if (rejectOption) {
      // rejected=true tells the backend to persist "rejected" status without
      // treating this as a dialog cancellation (cancelled=true would race with
      // the EventTypePermissionCancelled → "expired" update path).
      handleRespond(rejectOption.option_id, false, true);
    } else {
      handleRespond("", true);
    }
  }, [permissionMetadata, handleRespond]);

  return { isResponding, handleApprove, handleAllowAlways, hasAllowAlways, handleReject };
}
