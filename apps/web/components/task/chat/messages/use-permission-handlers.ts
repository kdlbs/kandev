"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { t } from "@/lib/i18n";
import { toast } from "@/lib/toast/sonner";
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
  request_id?: string;
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
  const storedStatus = permissionMetadata?.status;
  const missingRequestIdentity =
    !!permissionMessage &&
    (!storedStatus || storedStatus === "pending") &&
    !permissionMetadata?.request_id;
  const permissionStatus = missingRequestIdentity ? "expired" : storedStatus;
  const isPermissionPending =
    !!permissionMessage &&
    !!permissionMetadata?.request_id &&
    (!storedStatus || storedStatus === "pending");
  return { permissionMetadata, permissionStatus, isPermissionPending };
}

export function resolvePermissionAvailability(
  permissionStatus: PermissionRequestMetadata["status"],
  isPermissionPending: boolean,
  isUnavailable: boolean,
): Pick<ParsedPermission, "permissionStatus" | "isPermissionPending"> {
  if (!isUnavailable) return { permissionStatus, isPermissionPending };
  return { permissionStatus: "expired", isPermissionPending: false };
}

function isStalePermissionResponse(error: unknown): boolean {
  if (!(error instanceof Error)) return false;
  return ["permission_not_found", "permission_stale", "permission_already_resolved"].some((code) =>
    error.message.includes(code),
  );
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
  const [isUnavailable, setIsUnavailable] = useState(false);
  const requestId = permissionMetadata?.request_id;
  const currentRequestIdRef = useRef(requestId);

  useEffect(() => {
    currentRequestIdRef.current = requestId;
    setIsResponding(false);
    setIsUnavailable(false);
  }, [requestId]);

  const handleRespond = useCallback(
    async (optionId: string, cancelled: boolean = false, rejected: boolean = false) => {
      if (!permissionMessage) return;
      if (!requestId) {
        setIsUnavailable(true);
        toast.warning(t("task:permissionRequestNoLongerAvailable"));
        return;
      }
      const client = getWebSocketClient();
      if (!client) {
        console.error("WebSocket client not available");
        return;
      }
      setIsResponding(true);
      try {
        await client.request("permission.respond", {
          task_id: permissionMessage.task_id,
          session_id: permissionMessage.session_id,
          request_id: requestId,
          pending_id: permissionMetadata.pending_id,
          option_id: cancelled ? undefined : optionId,
          cancelled,
          rejected,
        });
      } catch (error) {
        console.error("Failed to respond to permission request:", error);
        if (currentRequestIdRef.current !== requestId) return;
        if (isStalePermissionResponse(error)) {
          setIsUnavailable(true);
          toast.warning(t("task:permissionRequestNoLongerAvailable"));
        } else {
          toast.error(t("task:permissionResponseFailed"));
        }
      } finally {
        if (currentRequestIdRef.current === requestId) {
          setIsResponding(false);
        }
      }
    },
    [permissionMessage, permissionMetadata, requestId],
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
  const allowAlwaysOption = useMemo(
    () => permissionMetadata?.options.find((opt) => opt.kind === "allow_always"),
    [permissionMetadata],
  );
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

  return {
    isResponding,
    isUnavailable,
    handleApprove,
    handleAllowAlways,
    hasAllowAlways,
    handleReject,
  };
}
