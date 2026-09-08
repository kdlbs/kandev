"use client";

import { IconAlertTriangle, IconCheck, IconX } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import { PermissionActionRow } from "./permission-action-row";
import { summarizePermissionAction } from "./permission-action-summary";
import {
  parsePermission,
  resolvePermissionAvailability,
  usePermissionResponseHandlers,
  type PermissionRequestMetadata,
} from "./use-permission-handlers";
import { t } from "@/lib/i18n";

function getPermissionStatusBadge(status: PermissionRequestMetadata["status"]) {
  switch (status) {
    case "approved":
      return (
        <span className="inline-flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
          <IconCheck className="h-3 w-3" /> {t("task:approved")}
        </span>
      );
    case "rejected":
      return (
        <span className="inline-flex items-center gap-1 text-xs text-red-600 dark:text-red-400">
          <IconX className="h-3 w-3" /> {t("task:rejected")}
        </span>
      );
    case "expired":
      return (
        <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
          {t("task:expired")}
        </span>
      );
    default:
      return (
        <span className="inline-flex items-center gap-1 text-xs text-amber-600 dark:text-amber-400">
          {t("task:pendingApproval")}
        </span>
      );
  }
}

type PermissionRequestMessageProps = {
  comment: Message;
};

export function PermissionRequestMessage({ comment }: PermissionRequestMessageProps) {
  const { permissionMetadata, permissionStatus, isPermissionPending } = parsePermission(comment);
  const {
    isResponding,
    isUnavailable,
    handleApprove,
    handleAllowAlways,
    hasAllowAlways,
    handleReject,
  } = usePermissionResponseHandlers({
    permissionMetadata,
    permissionMessage: comment,
  });

  const {
    permissionStatus: effectivePermissionStatus,
    isPermissionPending: effectivePermissionPending,
  } = resolvePermissionAvailability(permissionStatus, isPermissionPending, isUnavailable);
  const statusBadge = getPermissionStatusBadge(effectivePermissionStatus);
  const titleText = comment.content || t("task:permissionRequired");
  const detailSummary = summarizePermissionAction(permissionMetadata?.action_details, titleText);

  return (
    <div className="w-full">
      <div className="flex items-start gap-3 w-full">
        <div className="flex-shrink-0 mt-0.5">
          <IconAlertTriangle
            className={cn(
              "h-4 w-4",
              effectivePermissionPending
                ? "text-amber-600 dark:text-amber-400"
                : "text-muted-foreground",
            )}
          />
        </div>

        <div className="flex-1 min-w-0 pt-0.5">
          <div className="flex items-center gap-2 text-xs">
            <span
              className={cn(
                "font-mono text-xs",
                effectivePermissionPending
                  ? "text-amber-600 dark:text-amber-400"
                  : "text-muted-foreground",
              )}
            >
              {titleText}
            </span>
            {statusBadge}
          </div>

          {detailSummary && (
            <div
              className="mt-1 font-mono text-xs text-muted-foreground break-all"
              data-testid="permission-action-detail"
            >
              {detailSummary}
            </div>
          )}

          {effectivePermissionPending && (
            <div className="mt-2">
              <PermissionActionRow
                onApprove={handleApprove}
                onReject={handleReject}
                onAllowAlways={hasAllowAlways ? handleAllowAlways : undefined}
                isResponding={isResponding}
              />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
