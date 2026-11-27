"use client";

import { useId } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { automationRunPresentation } from "@/components/runs/automation-run-presentation";
import { STATE_LABEL_KEY } from "@/components/runs/automation-rows";
import { shortcutActivityLabel } from "@/components/app-sidebar/shortcut-activity-indicator";
import type { ShortcutActivity } from "@/hooks/domains/sidebar/use-shortcut-activity";
import type { ProjectedShortcut } from "@/lib/sidebar/layout-projection";
import { cn } from "@/lib/utils";

type MobileAutomationRowProps = {
  shortcut: Pick<ProjectedShortcut, "id" | "label" | "href" | "available">;
  activity?: ShortcutActivity;
  onNavigate: () => void;
};

function rowPresentation(activity: ShortcutActivity | undefined, t: (key: string) => string) {
  if (!activity || activity.loading || activity.error || activity.state === "unknown") {
    const label = activity ? shortcutActivityLabel(activity, t) : "";
    return { dotClass: "bg-muted-foreground/40", labels: label ? [label] : [], description: label };
  }
  const presentation = automationRunPresentation(activity.state, activity.lastRun);
  const labels: string[] = [];
  if (activity.state !== "idle" || !activity.lastRun)
    labels.push(t(STATE_LABEL_KEY[activity.state]));
  if (presentation.outcomeLabelKey && (activity.state === "idle" || presentation.showOutcome)) {
    labels.push(t(presentation.outcomeLabelKey));
  }
  return {
    dotClass: presentation.dotClass,
    labels,
    description: presentation.labelKeys.map((key) => `${t(key)}.`).join(" "),
  };
}

export function MobileAutomationRow({ shortcut, activity, onNavigate }: MobileAutomationRowProps) {
  const { t } = useTranslation();
  const descriptionId = useId();
  const { dotClass, labels, description } = rowPresentation(activity, t);
  const content = (
    <>
      <span aria-hidden="true" className={cn("h-1.5 w-1.5 shrink-0 rounded-full", dotClass)} />
      <span className="min-w-0 flex-1 truncate text-left" data-testid="mobile-automation-name">
        {shortcut.label}
      </span>
      <span
        aria-hidden="true"
        className="min-w-0 max-w-[60%] whitespace-normal wrap-anywhere text-right text-[11px] text-muted-foreground"
        data-testid="mobile-automation-status"
      >
        {labels.map((label) => (
          <span className="block" key={label}>
            {label}
          </span>
        ))}
      </span>
    </>
  );
  const rowProps = {
    className: cn(
      "flex min-h-11 min-w-0 w-full items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors",
      shortcut.available
        ? "cursor-pointer text-muted-foreground hover:bg-muted/60 hover:text-foreground"
        : "cursor-not-allowed text-muted-foreground/50",
    ),
    "aria-label": shortcut.label,
    "aria-describedby": description ? descriptionId : undefined,
    "data-testid": `sidebar-shortcut-${shortcut.id}`,
  };
  return (
    <>
      {shortcut.available && shortcut.href ? (
        <Link {...rowProps} href={shortcut.href} onClick={onNavigate}>
          {content}
        </Link>
      ) : (
        <span {...rowProps} aria-disabled="true">
          {content}
        </span>
      )}
      {description && (
        <span id={descriptionId} className="sr-only">
          {description}
        </span>
      )}
    </>
  );
}
