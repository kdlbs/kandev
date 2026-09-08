"use client";

import * as React from "react";
import { Dialog as DialogPrimitive } from "radix-ui";

import { cn } from "./lib/utils";
import { Button } from "./button";
import { IconX } from "@tabler/icons-react";
import { handleDialogDefaultActionKeyDown } from "./lib/dialog-default-action";
import {
  DIALOG_CLOSE_RECOVERY_MS,
  finishClosingDialogAnimations,
  subscribeDialogCloseRecovery,
} from "./lib/dialog-body-lock";

function Dialog({ onOpenChange, ...props }: React.ComponentProps<typeof DialogPrimitive.Root>) {
  const finishClosingAnimations = React.useCallback(
    () => finishClosingDialogAnimations(document),
    [],
  );

  // Unmount is one way to strand the lock — Radix's animation-based cleanup
  // never runs when the dialog disappears mid-close.
  React.useEffect(
    () => () => {
      finishClosingAnimations();
    },
    [finishClosingAnimations],
  );

  // Becoming visible again is the other. Dialog roots share one document-level
  // listener because the body lock and closing portals are document-global.
  React.useEffect(() => subscribeDialogCloseRecovery(document), []);

  // And the general case: once a close has been requested, the page must be
  // usable shortly afterwards whether or not animationend ever arrives.
  const handleOpenChange = React.useCallback(
    (open: boolean) => {
      onOpenChange?.(open);
      if (!open) window.setTimeout(finishClosingAnimations, DIALOG_CLOSE_RECOVERY_MS);
    },
    [finishClosingAnimations, onOpenChange],
  );

  return <DialogPrimitive.Root data-slot="dialog" onOpenChange={handleOpenChange} {...props} />;
}

function DialogTrigger({ ...props }: React.ComponentProps<typeof DialogPrimitive.Trigger>) {
  return <DialogPrimitive.Trigger data-slot="dialog-trigger" {...props} />;
}

function DialogPortal({ ...props }: React.ComponentProps<typeof DialogPrimitive.Portal>) {
  return <DialogPrimitive.Portal data-slot="dialog-portal" {...props} />;
}

function DialogClose({ ...props }: React.ComponentProps<typeof DialogPrimitive.Close>) {
  return <DialogPrimitive.Close data-slot="dialog-close" {...props} />;
}

function DialogOverlay({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Overlay>) {
  return (
    <DialogPrimitive.Overlay
      data-slot="dialog-overlay"
      className={cn(
        "bg-background/70 data-open:animate-in data-closed:animate-out data-closed:fade-out-0 data-open:fade-in-0 duration-100 fixed inset-0 isolate z-50",
        className,
      )}
      {...props}
    />
  );
}

function DialogContent({
  className,
  children,
  showCloseButton = true,
  overlayClassName,
  enterConfirms = true,
  onKeyDown,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Content> & {
  showCloseButton?: boolean;
  overlayClassName?: string;
  /** Pressing Enter activates the semantic action button. Default: true. */
  enterConfirms?: boolean;
}) {
  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    onKeyDown?.(event);
    if (enterConfirms) handleDialogDefaultActionKeyDown(event);
  };
  return (
    <DialogPortal>
      <DialogOverlay className={overlayClassName} />
      <DialogPrimitive.Content
        data-slot="dialog-content"
        className={cn(
          "border-border bg-popover text-popover-foreground data-open:animate-in data-closed:animate-out data-closed:fade-out-0 data-open:fade-in-0 data-closed:zoom-out-95 data-open:zoom-in-95 grid max-w-[calc(100%-2rem)] gap-4 rounded-lg border p-4 text-xs/relaxed shadow-md duration-100 sm:max-w-lg fixed top-1/2 left-1/2 z-50 w-full -translate-x-1/2 -translate-y-1/2",
          className,
        )}
        onKeyDown={handleKeyDown}
        {...props}
      >
        {children}
        {showCloseButton && (
          <DialogPrimitive.Close data-slot="dialog-close" asChild>
            <Button variant="ghost" className="absolute top-2 right-2" size="icon-sm">
              <IconX />
              <span className="sr-only">Close</span>
            </Button>
          </DialogPrimitive.Close>
        )}
      </DialogPrimitive.Content>
    </DialogPortal>
  );
}

function DialogHeader({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div data-slot="dialog-header" className={cn("gap-1 flex flex-col", className)} {...props} />
  );
}

function DialogFooter({
  className,
  showCloseButton = false,
  children,
  ...props
}: React.ComponentProps<"div"> & {
  showCloseButton?: boolean;
}) {
  return (
    <div
      data-slot="dialog-footer"
      className={cn("gap-2 flex flex-col-reverse sm:flex-row sm:justify-end", className)}
      {...props}
    >
      {children}
      {showCloseButton && (
        <DialogPrimitive.Close asChild>
          <Button variant="outline">Close</Button>
        </DialogPrimitive.Close>
      )}
    </div>
  );
}

function DialogTitle({ className, ...props }: React.ComponentProps<typeof DialogPrimitive.Title>) {
  return (
    <DialogPrimitive.Title
      data-slot="dialog-title"
      className={cn("min-w-0 text-sm font-medium text-balance wrap-anywhere", className)}
      {...props}
    />
  );
}

function DialogDescription({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Description>) {
  return (
    <DialogPrimitive.Description
      data-slot="dialog-description"
      className={cn(
        "text-muted-foreground min-w-0 *:[a]:hover:text-foreground text-xs/relaxed text-pretty wrap-anywhere *:[a]:underline *:[a]:underline-offset-3",
        className,
      )}
      {...props}
    />
  );
}

export {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
  DialogTrigger,
};
