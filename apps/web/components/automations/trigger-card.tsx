"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  IconClock,
  IconBrandGithub,
  IconWebhook,
  IconTrash,
  IconChevronDown,
  IconChevronUp,
  IconInfoCircle,
} from "@tabler/icons-react";
import type { AutomationTrigger, TriggerType } from "@/lib/types/automation";
import { ScheduledConfig } from "./trigger-configs/scheduled-config";
import { GitHubPRConfig } from "./trigger-configs/github-pr-config";
import { GitHubPushConfig } from "./trigger-configs/github-push-config";
import { GitHubCIConfig } from "./trigger-configs/github-ci-config";
import { WebhookConfig } from "./trigger-configs/webhook-config";

type TriggerCardProps = {
  trigger: AutomationTrigger;
  savedTrigger?: AutomationTrigger;
  automationId: string | null;
  workspaceId: string;
  onUpdate: (config: Record<string, unknown>) => void;
  onToggleEnabled: (enabled: boolean) => void;
  onDelete: () => void;
};

const TRIGGER_ICON: Record<TriggerType, typeof IconClock> = {
  scheduled: IconClock,
  github_pr: IconBrandGithub,
  github_push: IconBrandGithub,
  github_ci: IconBrandGithub,
  webhook: IconWebhook,
};

const TRIGGER_COLOR: Record<TriggerType, string> = {
  scheduled: "text-blue-400",
  github_pr: "text-purple-400",
  github_push: "text-purple-400",
  github_ci: "text-purple-400",
  webhook: "text-orange-400",
};

// Keyed by the cron expression the backend parses — syntax, never translated.
// Only the value is copy, so these hold catalog keys resolved at render.
const CRON_PRESET_KEYS: Record<string, string> = {
  "@hourly": "automations:cronEveryHour",
  "0 * * * *": "automations:cronEveryHour",
  "@daily": "automations:cronEveryDay",
  "0 0 * * *": "automations:cronEveryDay",
  "@weekly": "automations:cronEveryWeek",
  "0 0 * * 0": "automations:cronEveryWeek",
};

// Keyed by the persisted TriggerType.
const SIMPLE_SUMMARY_KEYS: Partial<Record<TriggerType, string>> = {
  github_push: "automations:summaryPushToBranch",
  github_ci: "automations:summaryCiCompleted",
  webhook: "automations:summaryWebhook",
};

const TRIGGER_INFO_KEYS: Record<TriggerType, string> = {
  scheduled: "automations:triggerInfoScheduled",
  github_pr: "automations:triggerInfoGithubPr",
  github_push: "automations:triggerInfoNotImplemented",
  github_ci: "automations:triggerInfoNotImplemented",
  webhook: "automations:triggerInfoWebhook",
};

// A plain function returning copy is invisible to `i18next/no-literal-string`,
// which only inspects literals in JSX — `t` is threaded in from the caller so
// the summary re-resolves on a locale switch.
function getTriggerSummary(
  trigger: AutomationTrigger,
  t: (key: string, values?: Record<string, unknown>) => string,
): string {
  const simple = SIMPLE_SUMMARY_KEYS[trigger.type];
  if (simple) return t(simple);

  const cfg = trigger.config;
  if (trigger.type === "scheduled") {
    const expr = (cfg.cron_expression as string) ?? "";
    const presetKey = CRON_PRESET_KEYS[expr];
    if (presetKey) return t(presetKey);
    // The expression itself is syntax and travels as an interpolated value.
    return expr
      ? t("automations:summaryCron", { expression: expr })
      : t("automations:summaryCustomSchedule");
  }
  if (trigger.type === "github_pr") {
    // Event names are the persisted GitHub event ids, not copy.
    const events = (cfg.events as string[]) ?? [];
    return events.length > 0
      ? t("automations:summaryPrEvents", { events: events.join(", ") })
      : t("automations:summaryPullRequestEvent");
  }
  // Fallback for a trigger type this build does not know: the raw persisted id.
  return trigger.type;
}

export function TriggerCard({
  trigger,
  savedTrigger,
  automationId,
  workspaceId,
  onUpdate,
  onToggleEnabled,
  onDelete,
}: TriggerCardProps) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const Icon = TRIGGER_ICON[trigger.type];
  const color = TRIGGER_COLOR[trigger.type];
  const isDirty =
    !savedTrigger ||
    trigger.enabled !== savedTrigger.enabled ||
    JSON.stringify(trigger.config) !== JSON.stringify(savedTrigger.config);

  return (
    <div
      className="rounded-lg border bg-card"
      data-settings-dirty={isDirty}
      data-settings-dirty-level="container"
    >
      <div className="flex items-center gap-3 px-4 py-3">
        <Icon className={`h-4 w-4 ${color} shrink-0`} />
        <button
          className="flex-1 text-sm text-left cursor-pointer hover:underline"
          onClick={() => setExpanded(!expanded)}
        >
          {getTriggerSummary(trigger, t)}
        </button>
        <Tooltip>
          <TooltipTrigger asChild>
            <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
          </TooltipTrigger>
          <TooltipContent>{t(TRIGGER_INFO_KEYS[trigger.type])}</TooltipContent>
        </Tooltip>
        <Button
          variant="ghost"
          size="icon-sm"
          className="cursor-pointer"
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? (
            <IconChevronUp className="h-3.5 w-3.5" />
          ) : (
            <IconChevronDown className="h-3.5 w-3.5" />
          )}
        </Button>
        <Switch
          size="sm"
          checked={trigger.enabled}
          data-settings-dirty={!savedTrigger || trigger.enabled !== savedTrigger.enabled}
          onCheckedChange={onToggleEnabled}
          className="cursor-pointer"
        />
        <Button variant="ghost" size="icon-sm" className="cursor-pointer" onClick={onDelete}>
          <IconTrash className="h-3.5 w-3.5 text-destructive" />
        </Button>
      </div>
      {expanded && (
        <div className="px-4 pb-4 pt-1 border-t">
          <TriggerConfigForm
            trigger={trigger}
            automationId={automationId}
            workspaceId={workspaceId}
            onUpdate={onUpdate}
          />
        </div>
      )}
    </div>
  );
}

function TriggerConfigForm({
  trigger,
  automationId,
  workspaceId,
  onUpdate,
}: {
  trigger: AutomationTrigger;
  automationId: string | null;
  workspaceId: string;
  onUpdate: (config: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  switch (trigger.type) {
    case "scheduled":
      return <ScheduledConfig config={trigger.config} onUpdate={onUpdate} />;
    case "github_pr":
      return <GitHubPRConfig config={trigger.config} onUpdate={onUpdate} />;
    case "github_push":
      return <GitHubPushConfig config={trigger.config} onUpdate={onUpdate} />;
    case "github_ci":
      return <GitHubCIConfig config={trigger.config} onUpdate={onUpdate} />;
    case "webhook":
      return <WebhookConfig automationId={automationId} workspaceId={workspaceId} />;
    default:
      return <p className="text-sm text-muted-foreground">{t("automations:unknownTriggerType")}</p>;
  }
}
