"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { t } from "@/lib/i18n";
import { toast } from "@/lib/toast/sonner";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { Message } from "@/lib/types/http";
import type { PermissionActionType } from "@/lib/types/permission";
import {
  allowAlwaysDecision,
  approveDecision,
  buildPermissionRespondRequest,
  denyDecision,
  isStalePermissionResponse,
  offeredChoiceDecision,
  offeredChoices as toOfferedChoices,
  type PermissionActionChoice,
  type PermissionDecision,
  type PermissionOption,
} from "@/lib/permissions/respond";

export type { PermissionActionChoice, PermissionOption };

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

type RespondPermission = (decision: PermissionDecision) => Promise<void>;

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
    async (decision: PermissionDecision) => {
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
        await client.request(
          "permission.respond",
          buildPermissionRespondRequest(
            {
              task_id: permissionMessage.task_id,
              session_id: permissionMessage.session_id,
              request_id: requestId,
              pending_id: permissionMetadata.pending_id,
            },
            decision,
          ),
        );
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

  const handleApprove = useCallback(() => {
    const decision = approveDecision(permissionMetadata?.options ?? []);
    if (decision) handleRespond(decision);
  }, [permissionMetadata, handleRespond]);

  // Only some agents offer allow_always (Cursor does); hasAllowAlways gates the button.
  const allowAlways = useMemo(
    () => allowAlwaysDecision(permissionMetadata?.options ?? []),
    [permissionMetadata],
  );
  const hasAllowAlways = !!allowAlways;
  const handleAllowAlways = useCallback(() => {
    if (allowAlways) handleRespond(allowAlways);
  }, [allowAlways, handleRespond]);

  const handleReject = useCallback(() => {
    handleRespond(denyDecision(permissionMetadata?.options ?? []));
  }, [permissionMetadata, handleRespond]);

  const { offeredChoices, handleOfferedChoice } = useOfferedDecisionHandlers(
    permissionMetadata,
    handleRespond,
  );

  return {
    isResponding,
    isUnavailable,
    handleApprove,
    handleAllowAlways,
    hasAllowAlways,
    handleReject,
    offeredChoices,
    handleOfferedChoice,
  };
}

function useOfferedDecisionHandlers(
  permissionMetadata: PermissionRequestMetadata | undefined,
  handleRespond: RespondPermission,
) {
  const offeredChoices: PermissionActionChoice[] = useMemo(
    () => toOfferedChoices(permissionMetadata?.options ?? []),
    [permissionMetadata],
  );
  const handleOfferedChoice = useCallback(
    (optionId: string) => {
      const decision = offeredChoiceDecision(permissionMetadata?.options ?? [], optionId);
      if (decision) handleRespond(decision);
    },
    [permissionMetadata, handleRespond],
  );
  return { offeredChoices, handleOfferedChoice };
}
