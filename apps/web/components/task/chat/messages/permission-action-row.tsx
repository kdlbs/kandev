"use client";

import { memo } from "react";
import { IconCheck, IconChecks, IconX } from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { Button } from "@kandev/ui/button";

type PermissionActionRowProps = {
  onApprove: () => void;
  onReject: () => void;
  // onAllowAlways is provided only when the agent offers an allow_always
  // option (e.g. Cursor). When undefined the "Always allow" button is hidden,
  // preserving the plain Approve/Deny row for agents without it.
  onAllowAlways?: () => void;
  isResponding?: boolean;
};

const ACTION_BUTTON_CLASS =
  "h-6 px-3 text-foreground border-border bg-background hover:bg-muted hover:border-foreground/40 transition-colors cursor-pointer";

export const PermissionActionRow = memo(function PermissionActionRow({
  onApprove,
  onReject,
  onAllowAlways,
  isResponding = false,
}: PermissionActionRowProps) {
  return (
    <div
      className="flex flex-wrap items-center gap-2 px-3 py-2  rounded-sm bg-amber-500/10"
      data-testid="permission-action-row"
    >
      <span className="text-xs text-amber-600 dark:text-amber-400 flex-1 min-w-0">
        Approve this action?
      </span>
      <Button
        size="xs"
        variant="outline"
        onClick={onReject}
        disabled={isResponding}
        data-testid="permission-reject"
        className={ACTION_BUTTON_CLASS}
      >
        <IconX className="h-4 w-4 mr-1 text-red-500" />
        Deny
      </Button>
      <Button
        size="xs"
        variant="outline"
        onClick={onApprove}
        disabled={isResponding}
        data-testid="permission-approve"
        className={ACTION_BUTTON_CLASS}
      >
        {isResponding ? (
          <GridSpinner className="text-foreground mr-1" />
        ) : (
          <IconCheck className="h-4 w-4 mr-1 text-green-500" />
        )}
        Approve
      </Button>
      {onAllowAlways && (
        <Button
          size="xs"
          variant="outline"
          onClick={onAllowAlways}
          disabled={isResponding}
          data-testid="permission-allow-always"
          className={ACTION_BUTTON_CLASS}
        >
          <IconChecks className="h-4 w-4 mr-1 text-green-500" />
          Always allow
        </Button>
      )}
    </div>
  );
});
