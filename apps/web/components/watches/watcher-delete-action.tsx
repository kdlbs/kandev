"use client";

import { useRef, useState, type ReactNode } from "react";
import { IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";

type WatcherDeleteActionProps = {
  title: ReactNode;
  cancelLabel: ReactNode;
  confirmLabel: ReactNode;
  ariaLabel: string;
  tooltipLabel?: ReactNode;
  triggerTestId?: string;
  confirmTestId?: string;
  testId?: string;
  onConfirm: () => void | Promise<void>;
};

/**
 * Local delete confirmation used by watcher rows/cards.
 *
 * Fine pointers keep the initiating delete button as the popover anchor.
 * Coarse pointers replace the action region with one inline cancel/delete
 * group so a phone never stacks a second overlay on top of its row.
 */
export function WatcherDeleteAction({
  title,
  cancelLabel,
  confirmLabel,
  ariaLabel,
  tooltipLabel,
  triggerTestId,
  confirmTestId,
  testId = "watcher-delete-confirmation",
  onConfirm,
}: WatcherDeleteActionProps) {
  const { isFinePointer } = useResponsiveBreakpoint();
  const [open, setOpen] = useState(false);
  const anchorRef = useRef<HTMLButtonElement>(null);

  if (!isFinePointer && open) {
    return (
      <InlineConfirmActions
        density="touch"
        testId={testId}
        ariaLabel={ariaLabel}
        cancelLabel={cancelLabel}
        confirmLabel={confirmLabel}
        confirmTestId={confirmTestId}
        onCancel={() => setOpen(false)}
        onClose={() => setOpen(false)}
        onConfirm={onConfirm}
      />
    );
  }

  const trigger = (
    <Button
      ref={anchorRef}
      variant="ghost"
      size="sm"
      className={`p-0 text-red-500 hover:text-red-600 cursor-pointer ${
        isFinePointer ? "h-7 w-7" : "h-11 w-11"
      }`}
      aria-label={ariaLabel}
      data-testid={triggerTestId}
      onClick={(event) => {
        event.stopPropagation();
        setOpen(true);
      }}
    >
      <IconTrash className="h-3.5 w-3.5" />
    </Button>
  );

  return (
    <>
      {isFinePointer && tooltipLabel ? (
        <Tooltip>
          <TooltipTrigger asChild>{trigger}</TooltipTrigger>
          <TooltipContent>{tooltipLabel}</TooltipContent>
        </Tooltip>
      ) : (
        trigger
      )}
      {isFinePointer ? (
        <ActionConfirmPopover
          open={open}
          anchorRef={anchorRef}
          title={title}
          cancelLabel={cancelLabel}
          confirmLabel={confirmLabel}
          confirmTestId={confirmTestId}
          testId={testId}
          onOpenChange={setOpen}
          onCancel={() => setOpen(false)}
          onConfirm={onConfirm}
        />
      ) : null}
    </>
  );
}
