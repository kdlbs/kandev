"use client";

import { useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import type { ProjectedSidebarNode, ProjectedShortcut } from "@/lib/sidebar/layout-projection";
import type {
  ShortcutActivity,
  ShortcutActivityReader,
} from "@/hooks/domains/sidebar/use-shortcut-activity";
import { AppSidebarSection } from "./app-sidebar-section";
import {
  ShortcutIconStrip,
  ShortcutOverflowMenu,
  ShortcutRows,
  type ShortcutActivation,
} from "./shortcut-section-actions";
import { ShortcutActivityIndicator, shortcutActivityLabel } from "./shortcut-activity-indicator";
import { cn } from "@/lib/utils";

export type ShortcutSectionProps = {
  node: ProjectedSidebarNode;
  collapsed?: boolean;
  mobile?: boolean;
  getActivity: ShortcutActivityReader;
  onActivateShortcut?: ShortcutActivation;
  onNavigate?: () => void;
};

function shortcutAutomationIds(shortcuts: ProjectedShortcut[]): string[] {
  return shortcuts
    .filter((shortcut) => shortcut.target.kind === "automation")
    .map((shortcut) => shortcut.target.id);
}

function isRunning(activity: ShortcutActivity | undefined): boolean {
  return activity?.state === "running";
}

function MobileShortcutSection({
  node,
  getActivity,
  onActivateShortcut,
  onNavigate,
}: ShortcutSectionProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const activityDescriptionId = `mobile-shortcut-section-activity-${encodeURIComponent(useId())}`;
  const automationIds = shortcutAutomationIds(node.shortcuts);
  const running = automationIds.some((id) => isRunning(getActivity(id)));

  return (
    <Collapsible open={open} onOpenChange={setOpen} className="min-w-0">
      <div className="flex min-w-0 flex-col gap-2">
        <div className="flex min-h-11 min-w-0 items-center gap-2">
          <CollapsibleTrigger asChild>
            <button
              type="button"
              className="min-h-11 min-w-0 flex-1 cursor-pointer truncate rounded-md px-3 text-left text-sm font-medium hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              aria-label={node.label}
              aria-describedby={running ? activityDescriptionId : undefined}
              data-testid={`mobile-shortcut-section-toggle-${node.id}`}
            >
              <span className="flex items-center gap-2">
                <node.icon className="h-4 w-4 shrink-0" aria-hidden="true" />
                <span className="truncate">{node.label}</span>
                {open ? (
                  <IconChevronDown className="size-3.5 shrink-0 text-muted-foreground" />
                ) : (
                  <IconChevronRight className="size-3.5 shrink-0 text-muted-foreground" />
                )}
                {running && (
                  <ShortcutActivityIndicator
                    activity={{ state: "running", loading: false, error: false }}
                  />
                )}
              </span>
            </button>
          </CollapsibleTrigger>
          {running && (
            <span id={activityDescriptionId} className="sr-only">
              {shortcutActivityLabel({ state: "running", loading: false, error: false }, t)}
            </span>
          )}
        </div>
        <div className="min-w-0 overflow-x-auto">
          <ShortcutIconStrip
            shortcuts={node.shortcuts}
            activity={getActivity}
            mobile
            onActivate={onActivateShortcut}
            onNavigate={onNavigate}
          />
        </div>
      </div>
      <CollapsibleContent>
        <ShortcutRows
          shortcuts={node.shortcuts}
          activity={getActivity}
          mobile
          onActivate={onActivateShortcut}
          onNavigate={onNavigate}
        />
      </CollapsibleContent>
    </Collapsible>
  );
}

function DesktopRailShortcutSection({
  node,
  getActivity,
  onActivateShortcut,
  onNavigate,
}: ShortcutSectionProps) {
  const Icon = node.icon;
  const running = shortcutAutomationIds(node.shortcuts).some((id) => isRunning(getActivity(id)));
  return (
    <ShortcutOverflowMenu
      shortcuts={node.shortcuts}
      activity={getActivity}
      onActivate={onActivateShortcut}
      onNavigate={onNavigate}
      triggerLabel={node.label}
      menuLabel={node.label}
      triggerTestId={`sidebar-shortcut-rail-${node.id}`}
      triggerClassName="mx-auto h-9 w-9"
      triggerContent={
        <span className="relative flex">
          <Icon className="h-4 w-4" />
          {running && (
            <ShortcutActivityIndicator
              activity={{ state: "running", loading: false, error: false }}
            />
          )}
        </span>
      }
      aggregateActivity={running ? { state: "running", loading: false, error: false } : undefined}
    />
  );
}

export function ShortcutSection({
  node,
  collapsed = false,
  mobile = false,
  getActivity,
  onActivateShortcut,
  onNavigate,
}: ShortcutSectionProps) {
  const { t } = useTranslation();
  if (mobile) {
    return (
      <MobileShortcutSection
        node={node}
        getActivity={getActivity}
        onActivateShortcut={onActivateShortcut}
        onNavigate={onNavigate}
        mobile
      />
    );
  }
  if (collapsed) {
    return (
      <DesktopRailShortcutSection
        node={node}
        getActivity={getActivity}
        onActivateShortcut={onActivateShortcut}
        onNavigate={onNavigate}
      />
    );
  }

  return (
    <AppSidebarSection
      id={`sidebar-shortcuts:${node.id}`}
      label={node.label}
      collapsed={false}
      icon={node.icon}
      headerAction={
        <ShortcutIconStrip
          shortcuts={node.shortcuts}
          activity={getActivity}
          onActivate={onActivateShortcut}
          onNavigate={onNavigate}
        />
      }
      headerActionVisibility="always"
      collapsedSummary={node.shortcuts.length > 0 ? node.shortcuts.length : undefined}
      defaultExpanded={false}
    >
      <ShortcutRows
        shortcuts={node.shortcuts}
        activity={getActivity}
        onActivate={onActivateShortcut}
        onNavigate={onNavigate}
      />
      {node.shortcuts.length === 0 && (
        <span className={cn("px-2.5 py-1.5 text-[13px] text-muted-foreground")}>
          {t("common:noResults")}
        </span>
      )}
    </AppSidebarSection>
  );
}
