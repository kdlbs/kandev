"use client";

import * as React from "react";
import { Command as CommandPrimitive, useCommandState } from "cmdk";

import { cn } from "./lib/utils";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "./dialog";
import { InputGroup, InputGroupAddon } from "./input-group";
import { IconSearch, IconCheck } from "@tabler/icons-react";

type CommandListElement = React.ElementRef<typeof CommandPrimitive.List>;
type CommandListProps = React.ComponentPropsWithoutRef<typeof CommandPrimitive.List>;
type RefCleanup = void | (() => void);

function applyCommandListRef(
  ref: React.ForwardedRef<CommandListElement>,
  node: CommandListElement | null,
): RefCleanup {
  if (typeof ref === "function") return ref(node);
  if (ref) ref.current = node;
}

function Command({ className, ...props }: React.ComponentProps<typeof CommandPrimitive>) {
  return (
    <CommandPrimitive
      data-slot="command"
      className={cn(
        "bg-popover text-popover-foreground rounded-xl p-1 flex size-full flex-col overflow-hidden",
        className,
      )}
      {...props}
    />
  );
}

function CommandDialog({
  title = "Command Palette",
  description = "Search for a command to run...",
  children,
  className,
  showCloseButton = false,
  overlayClassName,
  ...props
}: React.ComponentProps<typeof Dialog> & {
  title?: string;
  description?: string;
  className?: string;
  showCloseButton?: boolean;
  overlayClassName?: string;
}) {
  return (
    <Dialog {...props}>
      <DialogContent
        className={cn("rounded-xl! p-0 overflow-hidden", className)}
        showCloseButton={showCloseButton}
        overlayClassName={overlayClassName}
      >
        <DialogHeader className="sr-only">
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  );
}

function CommandInput({
  className,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Input>) {
  return (
    <div data-slot="command-input-wrapper" className="p-1 pb-0">
      <InputGroup className="bg-input/20 dark:bg-input/30 h-8! border-0">
        <CommandPrimitive.Input
          data-slot="command-input"
          className={cn(
            "w-full text-xs/relaxed outline-hidden disabled:cursor-not-allowed disabled:opacity-50",
            className,
          )}
          {...props}
        />
        <InputGroupAddon>
          <IconSearch className="size-3.5 shrink-0 opacity-50" />
        </InputGroupAddon>
      </InputGroup>
    </div>
  );
}

const CommandList = React.forwardRef<CommandListElement, CommandListProps>(function CommandList(
  { className, ...props },
  ref,
) {
  const search = useCommandState((state) => state.search);
  const internalRef = React.useRef<CommandListElement | null>(null);
  const latestForwardedRef = React.useRef(ref);
  const assignedRef = React.useRef<React.ForwardedRef<CommandListElement>>(null);
  const assignedCleanup = React.useRef<(() => void) | undefined>(undefined);

  latestForwardedRef.current = ref;

  const detachAssignedRef = React.useCallback(() => {
    const cleanup = assignedCleanup.current;
    const forwardedRef = assignedRef.current;

    assignedCleanup.current = undefined;
    assignedRef.current = null;

    if (cleanup) {
      cleanup();
      return;
    }

    if (forwardedRef) applyCommandListRef(forwardedRef, null);
  }, []);

  const attachAssignedRef = React.useCallback(
    (node: CommandListElement, forwardedRef: typeof ref) => {
      assignedRef.current = forwardedRef;
      const cleanup = applyCommandListRef(forwardedRef, node);
      assignedCleanup.current = typeof cleanup === "function" ? cleanup : undefined;
    },
    [],
  );

  const setRefs = React.useCallback(
    (node: CommandListElement | null) => {
      if (internalRef.current === node) return;

      detachAssignedRef();
      internalRef.current = node;

      if (!node) return;

      attachAssignedRef(node, latestForwardedRef.current);

      return () => {
        if (internalRef.current !== node) return;
        internalRef.current = null;
        detachAssignedRef();
      };
    },
    [attachAssignedRef, detachAssignedRef],
  );

  React.useLayoutEffect(() => {
    const node = internalRef.current;
    if (!node || assignedRef.current === ref) return;

    detachAssignedRef();
    attachAssignedRef(node, ref);
  }, [attachAssignedRef, detachAssignedRef, ref]);

  React.useLayoutEffect(() => {
    if (internalRef.current) internalRef.current.scrollTop = 0;
  }, [search]);

  return (
    <CommandPrimitive.List
      ref={setRefs}
      data-slot="command-list"
      className={cn(
        "max-h-72 scroll-py-1 outline-none overflow-x-hidden overflow-y-auto",
        className,
      )}
      {...props}
    />
  );
});

CommandList.displayName = CommandPrimitive.List.displayName;

function CommandEmpty({
  className,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Empty>) {
  return (
    <CommandPrimitive.Empty
      data-slot="command-empty"
      className={cn("py-6 text-center text-xs/relaxed", className)}
      {...props}
    />
  );
}

function CommandGroup({
  className,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Group>) {
  return (
    <CommandPrimitive.Group
      data-slot="command-group"
      className={cn(
        "text-foreground [&_[cmdk-group-heading]]:text-muted-foreground overflow-hidden p-1 [&_[cmdk-group-heading]]:px-2.5 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:font-medium",
        className,
      )}
      {...props}
    />
  );
}

function CommandSeparator({
  className,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Separator>) {
  return (
    <CommandPrimitive.Separator
      data-slot="command-separator"
      className={cn("bg-border/50 -mx-1 my-1 h-px", className)}
      {...props}
    />
  );
}

function CommandItem({
  className,
  children,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Item>) {
  return (
    <CommandPrimitive.Item
      data-slot="command-item"
      className={cn(
        "data-selected:bg-muted data-selected:text-foreground data-selected:*:[svg]:text-foreground relative flex min-h-7 cursor-default items-center gap-2 rounded-md px-2.5 py-1.5 text-xs/relaxed outline-hidden select-none [&_svg:not([class*='size-'])]:size-3.5 [[data-slot=dialog-content]_&]:rounded-md group/command-item data-[disabled=true]:pointer-events-none data-[disabled=true]:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0",
        className,
      )}
      {...props}
    >
      {children}
      <IconCheck className="ml-auto opacity-0 group-has-[[data-slot=command-shortcut]]/command-item:hidden group-data-[checked=true]/command-item:opacity-100" />
    </CommandPrimitive.Item>
  );
}

function CommandShortcut({ className, ...props }: React.ComponentProps<"span">) {
  return (
    <span
      data-slot="command-shortcut"
      className={cn(
        "text-muted-foreground group-data-selected/command-item:text-foreground ml-auto text-[0.625rem] tracking-widest",
        className,
      )}
      {...props}
    />
  );
}

export {
  Command,
  CommandDialog,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandShortcut,
  CommandSeparator,
};
