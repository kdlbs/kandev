"use client";

import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { PermissionActionRow } from "@/components/task/chat/messages/permission-action-row";
import { summarizePermissionAction } from "@/components/task/chat/messages/permission-action-summary";
import type {
  PermissionActionDetails,
  PermissionRequestMetadata,
} from "@/components/task/chat/messages/use-permission-handlers";
import type { RelayPermission } from "@/lib/api/domains/coordinator-relay-api";
import {
  allowAlwaysDecision,
  approveDecision,
  buildPermissionRespondRequest,
  denyDecision,
  isStalePermissionResponse,
  offeredChoiceDecision,
  offeredChoices,
  type PermissionDecision,
  type PermissionOption,
} from "@/lib/permissions/respond";
import { toast } from "@/lib/toast/sonner";
import { getWebSocketClient } from "@/lib/ws/connection";

export type PermissionAnswerProps = {
  permission: RelayPermission;
  /** First click on a decision button. */
  onEngaged: () => void;
  /** Recorded, or the request is no longer available: the item collapses. */
  onDone: () => void;
};

type PermissionState = { inFlight: boolean; failed: PermissionDecision | null };

/** Sends one decision; a failure keeps the item expanded and remembers the decision for Try again. */
function usePermissionResponder(
  permission: RelayPermission,
  onEngaged: () => void,
  onDone: () => void,
) {
  const { t } = useTranslation();
  const [state, setState] = useState<PermissionState>({ inFlight: false, failed: null });
  const inFlightRef = useRef(false);
  const message = permission.message;
  const metadata = message.metadata as PermissionRequestMetadata | undefined;

  const send = useCallback(
    async (decision: PermissionDecision) => {
      if (inFlightRef.current) return;
      onEngaged();
      inFlightRef.current = true;
      setState({ inFlight: true, failed: null });
      const client = getWebSocketClient();
      const requestId = metadata?.request_id;
      try {
        if (!client || !requestId) throw new Error("permission response unavailable");
        await client.request(
          "permission.respond",
          buildPermissionRespondRequest(
            {
              task_id: message.task_id,
              session_id: message.session_id,
              request_id: requestId,
              pending_id: metadata.pending_id,
            },
            decision,
          ),
        );
        inFlightRef.current = false;
        onDone();
      } catch (error) {
        inFlightRef.current = false;
        if (isStalePermissionResponse(error)) {
          toast.warning(t("task:permissionRequestNoLongerAvailable"));
          onDone();
          return;
        }
        toast.error(t("task:permissionResponseFailed"));
        setState({ inFlight: false, failed: decision });
      }
    },
    [message, metadata, onDone, onEngaged, t],
  );

  return { ...state, send };
}

/**
 * The expanded permission card: the chat's title, action summary and decision
 * buttons, resolving through the chat's `permission.respond` request
 * (docs/specs/coordinator/system-design/relay.md "Permission card").
 */
export function PermissionAnswer({ permission, onEngaged, onDone }: PermissionAnswerProps) {
  const { t } = useTranslation();
  const { inFlight, failed, send } = usePermissionResponder(permission, onEngaged, onDone);
  const metadata = permission.message.metadata as PermissionRequestMetadata | undefined;
  const options: PermissionOption[] = metadata?.options ?? [];
  const title = permission.message.content || t("task:permissionRequired");
  const detail = summarizePermissionAction(
    metadata?.action_details as PermissionActionDetails | undefined,
    title,
  );
  const choices = offeredChoices(options);
  const approve = approveDecision(options);
  const allowAlways = allowAlwaysDecision(options);

  return (
    <div className="space-y-2" data-testid="permission-answer">
      <div className="font-mono text-xs">{title}</div>
      {detail && (
        <div
          className="font-mono text-xs text-muted-foreground break-all"
          data-testid="permission-action-detail"
        >
          {detail}
        </div>
      )}
      <PermissionActionRow
        onApprove={() => approve && void send(approve)}
        approveDisabled={!approve}
        onReject={() => void send(denyDecision(options))}
        onAllowAlways={allowAlways ? () => void send(allowAlways) : undefined}
        offeredChoices={choices}
        onChooseOfferedChoice={(optionId) => {
          const decision = offeredChoiceDecision(options, optionId);
          if (decision) void send(decision);
        }}
        isResponding={inFlight}
      />
      {failed && (
        <div className="flex flex-wrap items-center gap-2" data-testid="permission-answer-error">
          <span className="text-xs text-destructive">{t("task:permissionResponseFailed")}</span>
          <Button
            size="sm"
            variant="outline"
            className="cursor-pointer"
            onClick={() => void send(failed)}
            disabled={inFlight}
          >
            {t("coordinator:tryAgain")}
          </Button>
        </div>
      )}
    </div>
  );
}
