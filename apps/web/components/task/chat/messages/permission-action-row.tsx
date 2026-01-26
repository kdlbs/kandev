'use client';

import { IconCheck, IconLoader2, IconX } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';

type PermissionActionRowProps = {
  onApprove: () => void;
  onReject: () => void;
  isResponding?: boolean;
};

export function PermissionActionRow({
  onApprove,
  onReject,
  isResponding = false,
}: PermissionActionRowProps) {
  return (
    <div className="flex items-center gap-2 px-3 py-2  rounded-sm bg-amber-500/10">
      <span className="text-xs text-amber-600 dark:text-amber-400 flex-1">
        Approve this action?
      </span>
      <Button
        size="xs"
        variant="outline"
        onClick={onReject}
        disabled={isResponding}
        className="h-6 px-3 text-foreground border-border bg-background hover:bg-muted hover:border-foreground/40 transition-colors cursor-pointer"
      >
        <IconX className="h-4 w-4 mr-1 text-red-500" />
        Deny
      </Button>
      <Button
        size="xs"
        variant="outline"
        onClick={onApprove}
        disabled={isResponding}
        className="h-6 px-3 text-foreground border-border bg-background hover:bg-muted hover:border-foreground/40 transition-colors cursor-pointer"
      >
        {isResponding ? (
          <IconLoader2 className="h-4 w-4 mr-1 animate-spin" />
        ) : (
          <IconCheck className="h-4 w-4 mr-1 text-green-500" />
        )}
        Approve
      </Button>
    </div>
  );
}
