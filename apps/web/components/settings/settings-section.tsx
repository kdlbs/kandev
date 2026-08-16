"use client";

import type { ReactNode } from "react";
import { Separator } from "@kandev/ui/separator";
import { useSettingsTargetRegistration } from "./settings-target-provider";
import { SETTINGS_TYPOGRAPHY } from "./settings-typography";

type SettingsSectionProps = {
  icon?: ReactNode;
  title: string;
  titleTestId?: string;
  titleAccessory?: ReactNode;
  description?: string;
  action?: ReactNode;
  children: ReactNode;
  discoveryTargetId?: string;
  /**
   * Rule the heading off from the body. For a section that *is* its page — the
   * workspace Repositories and Workflows tabs, which have no page heading of
   * their own — so it carries the same line under its title that the sibling
   * tabs have under theirs.
   */
  divided?: boolean;
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
  divided = false,
}: SettingsSectionProps) {
  const registerTarget = useSettingsTargetRegistration(discoveryTargetId);
  return (
    <section ref={registerTarget} className="space-y-4">
      {/* Stack the action row under the title on narrow screens instead of
          squeezing both onto one line; shrink-0 keeps the actions from being
          compressed by a long description when side by side. */}
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h3 className={SETTINGS_TYPOGRAPHY.sectionTitle} data-testid={titleTestId}>
              {icon}
              {title}
            </h3>
            {titleAccessory}
          </div>
          {description && <p className={SETTINGS_TYPOGRAPHY.sectionDescription}>{description}</p>}
        </div>
        {action && <div className="w-full shrink-0 md:w-auto">{action}</div>}
      </div>
      {divided && <Separator />}
      {children}
    </section>
  );
}
