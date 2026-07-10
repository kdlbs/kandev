"use client";

import { IconAlertTriangle, IconCheck, IconX } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import { PermissionActionRow } from "./permission-action-row";
import { summarizePermissionAction } from "./permission-action-summary";
import {
  parsePermission,
  usePermissionResponseHandlers,
  type PermissionRequestMetadata,
} from "./use-permission-handlers";

function getPermissionStatusBadge(status: PermissionRequestMetadata["status"]) {
  switch (status) {
    case "approved":
      return (
        <span className="inline-flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
          <IconCheck className="h-3 w-3" /> Approved
        </span>
      );
    case "rejected":
      return (
        <span className="inline-flex items-center gap-1 text-xs text-red-600 dark:text-red-400">
          <IconX className="h-3 w-3" /> Rejected
        </span>
      );
    case "expired":
      return (
        <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
          Expired
        </span>
      );
    default:
      return (
        <span className="inline-flex items-center gap-1 text-xs text-amber-600 dark:text-amber-400">
          Pending Approval
        </span>
      );
  }
}

type PermissionRequestMessageProps = {
  comment: Message;
};

export function PermissionRequestMessage({ comment }: PermissionRequestMessageProps) {
  const { permissionMetadata, permissionStatus, isPermissionPending } = parsePermission(comment);
  const { isResponding, handleApprove, handleAllowAlways, hasAllowAlways, handleReject } =
    usePermissionResponseHandlers({
      permissionMetadata,
      permissionMessage: comment,
    });

  const statusBadge = getPermissionStatusBadge(permissionStatus);
  const titleText = comment.content || "Permission Required";
  const detailSummary = summarizePermissionAction(permissionMetadata?.action_details, titleText);

  return (
    <div className="w-full">
      <div className="flex items-start gap-3 w-full">
        <div className="flex-shrink-0 mt-0.5">
          <IconAlertTriangle
            className={cn(
              "h-4 w-4",
              isPermissionPending ? "text-amber-600 dark:text-amber-400" : "text-muted-foreground",
            )}
          />
        </div>

        <div className="flex-1 min-w-0 pt-0.5">
          <div className="flex items-center gap-2 text-xs">
            <span
              className={cn(
                "font-mono text-xs",
                isPermissionPending
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

          {isPermissionPending && (
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
