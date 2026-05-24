"use client";

import { useState } from "react";
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
  automationId: string | null;
  onUpdate: (config: Record<string, unknown>) => void;
  onToggleEnabled: (enabled: boolean) => void;
  onDelete: () => void;
  // oneTimeWebhookSecret is the plaintext secret from the create response,
  // shown unmasked in the webhook config when present.
  oneTimeWebhookSecret?: string | null;
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

const CRON_PRESETS: Record<string, string> = {
  "@hourly": "Every hour",
  "0 * * * *": "Every hour",
  "@daily": "Every day",
  "0 0 * * *": "Every day",
  "@weekly": "Every week",
  "0 0 * * 0": "Every week",
};

const SIMPLE_SUMMARIES: Partial<Record<TriggerType, string>> = {
  github_push: "Push to branch",
  github_ci: "CI completed",
  webhook: "Webhook",
};

const TRIGGER_INFO: Record<TriggerType, string> = {
  scheduled: "Checked every 30 seconds. Fires when the cron schedule matches.",
  github_pr: "Polls GitHub API on your schedule for PRs matching your filters.",
  github_push: "Not yet implemented.",
  github_ci: "Not yet implemented.",
  webhook: "Fires immediately when a POST request hits the webhook URL.",
};

function getTriggerSummary(trigger: AutomationTrigger): string {
  const simple = SIMPLE_SUMMARIES[trigger.type];
  if (simple) return simple;

  const cfg = trigger.config;
  if (trigger.type === "scheduled") {
    const expr = (cfg.cron_expression as string) ?? "";
    return CRON_PRESETS[expr] ?? (expr ? `Cron: ${expr}` : "Custom schedule");
  }
  if (trigger.type === "github_pr") {
    const events = (cfg.events as string[]) ?? [];
    return events.length > 0 ? `PR: ${events.join(", ")}` : "Pull request event";
  }
  return trigger.type;
}

export function TriggerCard({
  trigger,
  automationId,
  onUpdate,
  onToggleEnabled,
  onDelete,
  oneTimeWebhookSecret,
}: TriggerCardProps) {
  // Webhook configs open by default when there's a fresh secret to show —
  // otherwise a user who just created an automation has to hunt for it.
  const [expanded, setExpanded] = useState(
    trigger.type === "webhook" && !!oneTimeWebhookSecret,
  );
  const Icon = TRIGGER_ICON[trigger.type];
  const color = TRIGGER_COLOR[trigger.type];

  return (
    <div className="rounded-lg border bg-card">
      <div className="flex items-center gap-3 px-4 py-3">
        <Icon className={`h-4 w-4 ${color} shrink-0`} />
        <button
          className="flex-1 text-sm text-left cursor-pointer hover:underline"
          onClick={() => setExpanded(!expanded)}
        >
          {getTriggerSummary(trigger)}
        </button>
        <Tooltip>
          <TooltipTrigger asChild>
            <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
          </TooltipTrigger>
          <TooltipContent>{TRIGGER_INFO[trigger.type]}</TooltipContent>
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
            onUpdate={onUpdate}
            oneTimeWebhookSecret={oneTimeWebhookSecret}
          />
        </div>
      )}
    </div>
  );
}

function TriggerConfigForm({
  trigger,
  automationId,
  onUpdate,
  oneTimeWebhookSecret,
}: {
  trigger: AutomationTrigger;
  automationId: string | null;
  onUpdate: (config: Record<string, unknown>) => void;
  oneTimeWebhookSecret?: string | null;
}) {
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
      return (
        <WebhookConfig
          automationId={automationId}
          initialSecret={oneTimeWebhookSecret ?? null}
        />
      );
    default:
      return <p className="text-sm text-muted-foreground">Unknown trigger type</p>;
  }
}
