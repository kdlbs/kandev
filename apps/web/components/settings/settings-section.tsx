"use client";

import type { ReactNode } from "react";
import { useSettingsTargetRegistration } from "./settings-target-provider";

type SettingsSectionProps = {
  icon?: ReactNode;
  title: string;
  titleTestId?: string;
  titleAccessory?: ReactNode;
  description?: string;
  action?: ReactNode;
  children: ReactNode;
  discoveryTargetId?: string;
};

export function SettingsSection({
  icon,
  title,
  titleTestId,
  titleAccessory,
  description,
  action,
  children,
  discoveryTargetId,
}: SettingsSectionProps) {
  const registerTarget = useSettingsTargetRegistration(discoveryTargetId);
  return (
    <section ref={registerTarget} className="space-y-4">
      {/* Stack the action row under the title on narrow screens instead of
          squeezing both onto one line; shrink-0 keeps the actions from being
          compressed by a long description when side by side. */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="text-lg font-semibold flex items-center gap-2" data-testid={titleTestId}>
              {icon}
              {title}
            </h3>
            {titleAccessory}
          </div>
          {description && <p className="text-sm text-muted-foreground mt-1">{description}</p>}
        </div>
        {action && <div className="w-full shrink-0 sm:w-auto">{action}</div>}
      </div>
      {children}
    </section>
  );
}
