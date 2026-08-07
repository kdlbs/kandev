"use client";

import {
  forwardRef,
  useCallback,
  useRef,
  type ComponentPropsWithoutRef,
  type ReactNode,
} from "react";
import { IconGitPullRequest, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { cn } from "@kandev/ui/lib/utils";

export const CHANGE_REQUEST_STATUS_CHIP_CLASS =
  "cursor-pointer inline-flex items-center gap-1 rounded-md px-1 py-0.5 text-xs";

export const ChangeRequestStatusChip = forwardRef<
  HTMLButtonElement,
  ComponentPropsWithoutRef<"button">
>(function ChangeRequestStatusChip({ className, type = "button", ...props }, ref) {
  return (
    <button
      {...props}
      ref={ref}
      type={type}
      className={cn(CHANGE_REQUEST_STATUS_CHIP_CLASS, className)}
    />
  );
});

export const ChangeRequestTopbarButton = forwardRef<
  HTMLButtonElement,
  Omit<ComponentPropsWithoutRef<typeof Button>, "size" | "variant">
>(function ChangeRequestTopbarButton({ className, type = "button", ...props }, ref) {
  return (
    <Button
      {...props}
      ref={ref}
      type={type}
      size="sm"
      variant="outline"
      className={cn("cursor-pointer gap-1.5 px-2", className)}
    />
  );
});

export function ChangeRequestTopbarContent({
  label,
  colorClassName,
  statusIcon,
  dropdown,
}: {
  label: string;
  colorClassName: string;
  statusIcon?: ReactNode;
  dropdown?: ReactNode;
}) {
  return (
    <>
      <IconGitPullRequest className={cn("h-4 w-4", colorClassName)} />
      <span className="text-xs font-medium">{label}</span>
      {dropdown ?? statusIcon}
    </>
  );
}

type ChipHoverHandlers = {
  onTriggerEnter: () => void;
  onTriggerLeave: () => void;
};

export function ChangeRequestStatusChipHoverArea({
  handlers,
  children,
}: {
  handlers: ChipHoverHandlers;
  children: ReactNode;
}) {
  return (
    <span
      className="inline-flex"
      onMouseOver={handlers.onTriggerEnter}
      onMouseEnter={handlers.onTriggerEnter}
      onMouseMove={handlers.onTriggerEnter}
      onPointerOver={handlers.onTriggerEnter}
      onPointerEnter={handlers.onTriggerEnter}
      onPointerMove={handlers.onTriggerEnter}
      onMouseLeave={handlers.onTriggerLeave}
      onPointerLeave={handlers.onTriggerLeave}
      onFocus={handlers.onTriggerEnter}
      onBlur={handlers.onTriggerLeave}
    >
      {children}
    </span>
  );
}

export function useChangeRequestStatusChipTriggerGuard() {
  const ref = useRef<HTMLButtonElement>(null);
  const onPointerDownOutside = useCallback(
    (event: { target: EventTarget | null; preventDefault: () => void }) => {
      if (ref.current?.contains(event.target as Node)) event.preventDefault();
    },
    [],
  );
  return { ref, onPointerDownOutside };
}

export function ChangeRequestStatusDrawerContent({
  testId,
  headerTestId,
  closeTestId,
  bodyTestId,
  title,
  description,
  children,
  footer,
}: {
  testId: string;
  headerTestId?: string;
  closeTestId: string;
  bodyTestId?: string;
  title: ReactNode;
  description: string;
  children: ReactNode;
  footer?: ReactNode;
}) {
  return (
    <DrawerContent data-testid={testId} className="max-h-[80dvh] flex flex-col">
      <DrawerHeader
        data-testid={headerTestId}
        className="flex flex-row items-center justify-between border-b py-2"
      >
        <DrawerTitle className="text-sm">{title}</DrawerTitle>
        <DrawerDescription className="sr-only">{description}</DrawerDescription>
        <DrawerClose asChild>
          <Button
            data-testid={closeTestId}
            variant="ghost"
            size="icon-sm"
            aria-label="Close PR status"
            className="cursor-pointer"
          >
            <IconX className="h-4 w-4" />
          </Button>
        </DrawerClose>
      </DrawerHeader>
      <div
        data-testid={bodyTestId}
        className="flex-1 min-h-0 overflow-y-auto p-3"
        data-vaul-no-drag
      >
        {children}
      </div>
      {footer}
    </DrawerContent>
  );
}
