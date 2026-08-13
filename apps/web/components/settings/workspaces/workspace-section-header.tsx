"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { workspaceSettingsTabSpec, type WorkspaceSettingsTab } from "./workspace-settings-shell";

/**
 * The heading every workspace settings tab opens with.
 *
 * The six tabs used to head themselves three different ways: three repeated the
 * workspace's name — already the page title above the strip, so the tab you had
 * just clicked went unnamed — while the rest named the section, only one of them
 * with a mark. This is the shape they now share: the section's name and the mark
 * from its own tab, over a line saying what the section is for.
 *
 * Name and mark come from the tab table rather than from each page, so a page
 * cannot end up calling its section something the tab that opens it does not.
 */
export function WorkspaceSectionHeader({
  tab,
  description,
  action,
}: {
  tab: WorkspaceSettingsTab;
  description: string;
  /** Section-level control, e.g. Automations' "New automation". */
  action?: ReactNode;
}) {
  const { t } = useTranslation();
  const { labelKey, icon: Icon } = workspaceSettingsTabSpec(tab);

  return (
    <div
      className="flex flex-wrap items-start justify-between gap-4"
      data-testid={`workspace-section-header-${tab}`}
    >
      <div className="min-w-0">
        <h2 className="flex items-center gap-2 text-lg font-semibold">
          <Icon className="h-5 w-5 shrink-0" />
          {t(labelKey)}
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">{description}</p>
      </div>
      {action}
    </div>
  );
}
