"use client";

import { useId, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconDots } from "@tabler/icons-react";
import Link from "@/components/routing/app-link";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { ProjectedShortcut } from "@/lib/sidebar/layout-projection";
import type { ShortcutActivity } from "@/hooks/domains/sidebar/use-shortcut-activity";
import { cn } from "@/lib/utils";
import { ShortcutActivityIndicator, shortcutActivityLabel } from "./shortcut-activity-indicator";

export type ShortcutActivation = (shortcut: ProjectedShortcut) => void;
export type ShortcutActivityReader = (automationId: string) => ShortcutActivity | undefined;

type ShortcutActionProps = {
  shortcut: ProjectedShortcut;
  activity?: ShortcutActivity;
  mobile?: boolean;
  className?: string;
  onActivate?: ShortcutActivation;
  onNavigate?: () => void;
  showLabel?: boolean;
  descriptionSuffix?: string;
  children?: ReactNode;
};

function activityDescriptionId(shortcut: ProjectedShortcut, suffix: string): string {
  return `sidebar-shortcut-activity-${encodeURIComponent(shortcut.id)}-${suffix}`;
}

function ShortcutIcon({
  shortcut,
  activity,
}: {
  shortcut: ProjectedShortcut;
  activity?: ShortcutActivity;
}) {
  const Icon = shortcut.icon;
  return (
    <span className="relative flex shrink-0">
      <Icon className="h-4 w-4" aria-hidden="true" />
      {activity && <ShortcutActivityIndicator activity={activity} />}
    </span>
  );
}

function activityDescription(
  shortcut: ProjectedShortcut,
  activity: ShortcutActivity | undefined,
  t: (key: string) => string,
  descriptionSuffix: string,
) {
  if (!activity || shortcut.target.kind !== "automation") return null;
  return (
    <span id={activityDescriptionId(shortcut, descriptionSuffix)} className="sr-only">
      {shortcut.label}: {shortcutActivityLabel(activity, t)}
    </span>
  );
}

export function ShortcutAction({
  shortcut,
  activity,
  mobile = false,
  className,
  onActivate,
  onNavigate,
  showLabel = false,
  descriptionSuffix = "action",
  children,
}: ShortcutActionProps) {
  const { t } = useTranslation();
  const description = activityDescription(shortcut, activity, t, descriptionSuffix);
  const describedBy = description ? activityDescriptionId(shortcut, descriptionSuffix) : undefined;
  const baseClass = cn(
    "relative flex items-center gap-2 rounded-md text-left transition-colors",
    mobile ? "min-h-11 min-w-11 px-3" : "h-7 min-w-7 px-1.5",
    shortcut.available
      ? "cursor-pointer text-muted-foreground hover:bg-muted/60 hover:text-foreground"
      : "cursor-not-allowed text-muted-foreground/50",
    className,
  );
  const content = children ?? <ShortcutIcon shortcut={shortcut} activity={activity} />;

  if (!shortcut.available) {
    return (
      <>
        <span
          className={baseClass}
          aria-label={shortcut.label}
          aria-disabled="true"
          aria-describedby={describedBy}
          data-testid={`sidebar-shortcut-${shortcut.id}`}
        >
          {content}
          {!mobile && <span className="sr-only">{t("common:unavailable")}</span>}
        </span>
        {description}
      </>
    );
  }

  const trigger = shortcut.href ? (
    <Link
      href={shortcut.href}
      onClick={onNavigate}
      className={baseClass}
      aria-label={shortcut.label}
      aria-describedby={describedBy}
      data-testid={`sidebar-shortcut-${shortcut.id}`}
    >
      {content}
      {showLabel && <span className="min-w-0 truncate">{shortcut.label}</span>}
    </Link>
  ) : (
    <button
      type="button"
      className={baseClass}
      onClick={() => onActivate?.(shortcut)}
      aria-label={shortcut.label}
      aria-describedby={describedBy}
      data-testid={`sidebar-shortcut-${shortcut.id}`}
    >
      {content}
      {showLabel && <span className="min-w-0 truncate">{shortcut.label}</span>}
    </button>
  );

  return (
    <>
      {mobile ? (
        trigger
      ) : (
        <Tooltip>
          <TooltipTrigger asChild>{trigger}</TooltipTrigger>
          <TooltipContent side="right">{shortcut.label}</TooltipContent>
        </Tooltip>
      )}
      {description}
    </>
  );
}

type ShortcutIconStripProps = {
  shortcuts: ProjectedShortcut[];
  activity: ShortcutActivityReader;
  mobile?: boolean;
  onActivate?: ShortcutActivation;
  onNavigate?: () => void;
};

export function ShortcutIconStrip({
  shortcuts,
  activity,
  mobile = false,
  onActivate,
  onNavigate,
}: ShortcutIconStripProps) {
  const { t } = useTranslation();
  const inline = shortcuts.slice(0, 4);
  const overflow = shortcuts.slice(4);
  const overflowRunning = overflow.some(
    (shortcut) =>
      shortcut.target.kind === "automation" && activity(shortcut.target.id)?.state === "running",
  );
  return (
    <div className={cn("flex min-w-0 items-center", mobile ? "gap-1" : "gap-0.5")}>
      {inline.map((shortcut) => (
        <ShortcutAction
          key={shortcut.id}
          shortcut={shortcut}
          activity={
            shortcut.target.kind === "automation" ? activity(shortcut.target.id) : undefined
          }
          mobile={mobile}
          onActivate={onActivate}
          onNavigate={onNavigate}
          descriptionSuffix="header"
        />
      ))}
      {overflow.length > 0 && (
        <ShortcutOverflowMenu
          shortcuts={overflow}
          activity={activity}
          mobile={mobile}
          onActivate={onActivate}
          onNavigate={onNavigate}
          aggregateActivity={
            overflowRunning ? { state: "running", loading: false, error: false } : undefined
          }
        />
      )}
      {shortcuts.length === 0 && <span className="sr-only">{t("common:noResults")}</span>}
    </div>
  );
}

type ShortcutOverflowMenuProps = ShortcutIconStripProps & {
  shortcuts: ProjectedShortcut[];
  triggerLabel?: string;
  menuLabel?: string;
  triggerContent?: ReactNode;
  aggregateActivity?: ShortcutActivity;
  triggerClassName?: string;
  triggerTestId?: string;
};

export function ShortcutOverflowMenu({
  shortcuts,
  activity,
  mobile = false,
  onActivate,
  onNavigate,
  triggerLabel,
  menuLabel,
  triggerContent,
  aggregateActivity,
  triggerClassName,
  triggerTestId,
}: ShortcutOverflowMenuProps) {
  const { t } = useTranslation();
  const label = triggerLabel ?? t("common:showMoreActions");
  const activityDescriptionId = `sidebar-shortcut-aggregate-activity-${encodeURIComponent(useId())}`;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className={cn(
            "relative cursor-pointer",
            mobile ? "size-11" : "h-7 w-7",
            triggerClassName,
          )}
          aria-label={label}
          aria-describedby={aggregateActivity ? activityDescriptionId : undefined}
          data-testid={triggerTestId ?? "sidebar-shortcut-more"}
        >
          {triggerContent ?? (
            <span className="relative flex">
              <IconDots className="h-4 w-4" />
              {aggregateActivity && <ShortcutActivityIndicator activity={aggregateActivity} />}
            </span>
          )}
        </Button>
      </DropdownMenuTrigger>
      {aggregateActivity && (
        <span id={activityDescriptionId} className="sr-only">
          {shortcutActivityLabel(aggregateActivity, t)}
        </span>
      )}
      <DropdownMenuContent align="start" side={mobile ? "bottom" : "right"}>
        {menuLabel && <DropdownMenuLabel>{menuLabel}</DropdownMenuLabel>}
        {shortcuts.map((shortcut) => {
          const shortcutActivity =
            shortcut.target.kind === "automation" ? activity(shortcut.target.id) : undefined;
          const Icon = shortcut.icon;
          if (shortcut.href && shortcut.available) {
            return (
              <DropdownMenuItem key={shortcut.id} asChild className="cursor-pointer">
                <Link href={shortcut.href} onClick={onNavigate}>
                  <span className="relative flex shrink-0">
                    <Icon className="h-4 w-4" />
                    {shortcutActivity && <ShortcutActivityIndicator activity={shortcutActivity} />}
                  </span>
                  <span className="min-w-0 flex-1 truncate">{shortcut.label}</span>
                  {shortcutActivity && (
                    <span className="sr-only">{shortcutActivityLabel(shortcutActivity, t)}</span>
                  )}
                </Link>
              </DropdownMenuItem>
            );
          }
          return (
            <DropdownMenuItem
              key={shortcut.id}
              disabled={!shortcut.available}
              onSelect={() => onActivate?.(shortcut)}
            >
              <span className="relative flex shrink-0">
                <Icon className="h-4 w-4" />
                {shortcutActivity && <ShortcutActivityIndicator activity={shortcutActivity} />}
              </span>
              <span className="min-w-0 flex-1 truncate">{shortcut.label}</span>
              {shortcutActivity && (
                <span className="sr-only">{shortcutActivityLabel(shortcutActivity, t)}</span>
              )}
            </DropdownMenuItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

type ShortcutRowsProps = {
  shortcuts: ProjectedShortcut[];
  activity: ShortcutActivityReader;
  mobile?: boolean;
  onActivate?: ShortcutActivation;
  onNavigate?: () => void;
};

export function ShortcutRows({
  shortcuts,
  activity,
  mobile = false,
  onActivate,
  onNavigate,
}: ShortcutRowsProps) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-0.5" data-testid="shortcut-section-rows">
      {shortcuts.map((shortcut) => (
        <ShortcutAction
          key={shortcut.id}
          shortcut={shortcut}
          activity={
            shortcut.target.kind === "automation" ? activity(shortcut.target.id) : undefined
          }
          mobile={mobile}
          className={cn(!mobile && "w-full px-2.5")}
          onActivate={onActivate}
          onNavigate={onNavigate}
          descriptionSuffix="row"
          showLabel={mobile}
        >
          <span className="flex min-w-0 flex-1 items-center gap-2">
            <ShortcutIcon
              shortcut={shortcut}
              activity={
                shortcut.target.kind === "automation" ? activity(shortcut.target.id) : undefined
              }
            />
            <span className="min-w-0 flex-1 truncate">{shortcut.label}</span>
            {shortcut.target.kind === "automation" && activity(shortcut.target.id) && (
              <span className="shrink-0 text-[11px] text-muted-foreground">
                {shortcutActivityLabel(activity(shortcut.target.id)!, t)}
              </span>
            )}
          </span>
        </ShortcutAction>
      ))}
    </div>
  );
}
