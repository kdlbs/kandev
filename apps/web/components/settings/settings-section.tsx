"use client";

import type { ReactNode } from "react";
import { SettingsGroup } from "./settings-group";

type SettingsSectionProps = {
  icon?: ReactNode;
  title: string;
  titleTestId?: string;
  titleAccessory?: ReactNode;
  description?: string;
  action?: ReactNode;
  children: ReactNode;
  discoveryTargetId?: string;
  framed?: boolean;
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
  framed = true,
  divided = false,
}: SettingsSectionProps) {
  return (
    <SettingsGroup
      title={
        <span className="flex items-center gap-2">
          {icon}
          {title}
        </span>
      }
      titleTestId={titleTestId}
      titleAccessory={titleAccessory}
      description={description}
      action={action}
      discoveryTargetId={discoveryTargetId}
      frame={framed ? "card" : "none"}
      contentClassName={divided ? "border-t border-border/70" : undefined}
    >
      {children}
    </SettingsGroup>
  );
}
