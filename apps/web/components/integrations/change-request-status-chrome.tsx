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
import { t } from "@/lib/i18n";

export const CHANGE_REQUEST_STATUS_CHIP_CLASS =
  "cursor-pointer inline-flex items-center gap-1 rounded-md px-1 py-0.5 text-xs";

export const ChangeRequestStatusChip = forwardRef<
  HTMLButtonElement,
  ComponentPropsWithoutRef<"button">
>(function ChangeRequestStatusChip({ className, type = "button", ...props }, ref) {
  return (
    <button
      {...props}
      data-slot="change-request-status-chip"
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
  leadingIcon,
}: {
  label: string;
  colorClassName: string;
  statusIcon?: ReactNode;
  dropdown?: ReactNode;
  leadingIcon?: ReactNode;
}) {
  return (
    <>
      {leadingIcon ?? <IconGitPullRequest className={cn("h-4 w-4", colorClassName)} />}
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

export function useChangeRequestStatusChipTriggerGuard(externalRef?: {
  current: HTMLButtonElement | null;
}) {
  const fallbackRef = useRef<HTMLButtonElement>(null);
  const ref = externalRef ?? fallbackRef;
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
  closeLabel,
  fixedContent = false,
}: {
  testId: string;
  headerTestId?: string;
  closeTestId: string;
  bodyTestId?: string;
  title: ReactNode;
  description: string;
  children: ReactNode;
  footer?: ReactNode;
  closeLabel?: string;
  fixedContent?: boolean;
}) {
  return (
    <DrawerContent
      data-testid={testId}
      className={cn(
        "max-h-[80dvh] flex flex-col",
        fixedContent && [
          "h-[80dvh] data-[vaul-drawer-direction=bottom]:max-h-[80dvh]",
          "[@media(max-height:480px)]:h-dvh [@media(max-height:480px)]:max-h-dvh [@media(max-height:480px)]:data-[vaul-drawer-direction=bottom]:max-h-dvh [@media(max-height:480px)]:data-[vaul-drawer-direction=bottom]:mt-0",
          "[@media(max-height:480px)]:pt-[calc(0.5rem+env(safe-area-inset-top,0px))] [@media(max-height:480px)]:pr-[calc(0.5rem+env(safe-area-inset-right,0px))] [@media(max-height:480px)]:pl-[calc(0.5rem+env(safe-area-inset-left,0px))]",
          "[@media(max-height:480px)]:[&>div:first-child]:hidden [@media(max-height:480px)]:[&_[data-slot=change-request-popover-header]]:pr-12 [@media(max-height:480px)]:[&_[role=tablist]]:pr-12",
        ],
      )}
    >
      <DrawerHeader
        data-testid={headerTestId}
        className={cn(
          "flex shrink-0 flex-row items-center justify-between border-b py-2",
          fixedContent &&
            "[@media(max-height:480px)]:h-0 [@media(max-height:480px)]:border-0 [@media(max-height:480px)]:p-0",
        )}
      >
        <DrawerTitle
          className={cn("text-sm", fixedContent && "[@media(max-height:480px)]:sr-only")}
        >
          {title}
        </DrawerTitle>
        <DrawerDescription className="sr-only">{description}</DrawerDescription>
        <DrawerClose asChild>
          <Button
            data-testid={closeTestId}
            variant="ghost"
            size="icon-sm"
            aria-label={closeLabel ?? t("integrations:closePullRequestStatus")}
            className={cn(
              "cursor-pointer",
              fixedContent &&
                "min-h-12 min-w-12 [@media(max-height:480px)]:absolute [@media(max-height:480px)]:top-[calc(0.5rem+env(safe-area-inset-top,0px))] [@media(max-height:480px)]:right-[calc(0.5rem+env(safe-area-inset-right,0px))] [@media(max-height:480px)]:z-10",
            )}
          >
            <IconX className="h-4 w-4" />
          </Button>
        </DrawerClose>
      </DrawerHeader>
      <div
        data-testid={bodyTestId}
        className={cn(
          "flex-1 min-h-0 p-3",
          fixedContent
            ? "flex flex-col overflow-hidden pb-[max(0.75rem,env(safe-area-inset-bottom))] [@media(max-height:480px)]:p-2 [@media(max-height:480px)]:pb-[max(0.5rem,env(safe-area-inset-bottom))]"
            : "overflow-y-auto",
        )}
        data-vaul-no-drag
      >
        {children}
      </div>
      {footer}
    </DrawerContent>
  );
}
