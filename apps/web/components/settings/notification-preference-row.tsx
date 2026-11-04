"use client";

import type { ComponentProps, ReactNode } from "react";
import { SettingsFieldLabel } from "./settings-typography";
import { useSettingsTargetRegistration } from "./settings-target-provider";
import { UnsavedChangesBadge } from "./unsaved-indicator";
import { cn } from "@/lib/utils";

type NotificationPreferenceRowProps = Omit<ComponentProps<"div">, "title"> & {
  label: ReactNode;
  labelFor?: string;
  leadingControl?: ReactNode;
  footer?: ReactNode;
  discoveryTargetId?: string;
  isDirty?: boolean;
};

/**
 * One compact notification preference: the label keeps its own line on phones,
 * the leading control sits beside it there, and everything shares a desktop row.
 */
export function NotificationPreferenceRow({
  label,
  labelFor,
  leadingControl,
  footer,
  discoveryTargetId,
  isDirty = false,
  className,
  children,
  ...props
}: NotificationPreferenceRowProps) {
  const registerTarget = useSettingsTargetRegistration(discoveryTargetId);
  return (
    <div
      {...props}
      ref={registerTarget}
      data-notification-preference-row="true"
      data-settings-dirty={isDirty}
      className={cn(
        "min-w-0 rounded-lg border border-border/70 bg-card px-4 py-3",
        "md:py-2.5",
        className,
      )}
    >
      <div
        className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-2 md:flex md:gap-x-4"
        data-notification-preference-body="true"
      >
        <div className="flex min-w-0 flex-wrap items-center gap-2 md:flex-1">
          <SettingsFieldLabel htmlFor={labelFor}>{label}</SettingsFieldLabel>
          {isDirty && <UnsavedChangesBadge />}
        </div>
        {leadingControl && (
          <div className="flex shrink-0 items-center justify-end md:order-2">{leadingControl}</div>
        )}
        {children && (
          <div
            className="col-span-2 flex min-w-0 flex-wrap items-center gap-2 md:order-3 md:flex-nowrap md:justify-end"
            data-notification-preference-controls="true"
          >
            {children}
          </div>
        )}
      </div>
      {footer && (
        <div className="mt-2 md:mt-1" data-notification-preference-footer="true">
          {footer}
        </div>
      )}
    </div>
  );
}
