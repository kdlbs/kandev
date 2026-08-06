"use client";

import { useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useToast } from "@/components/toast-provider";
import { useAppStoreApi } from "@/components/state-provider";
import { getJiraTicket } from "@/lib/api/domains/jira-api";
import { getLinearIssue } from "@/lib/api/domains/linear-api";
import { getSentryIssue } from "@/lib/api/domains/sentry-api";
import { updateTask } from "@/lib/api/domains/kanban-api";
import { JIRA_KEY_RE } from "@/components/jira/jira-ticket-common";
import { LINEAR_KEY_RE } from "@/components/linear/linear-issue-common";
import { extractSentryShortId } from "@/components/sentry/sentry-issue-common";
import { useSentryInstances } from "@/hooks/domains/sentry/use-sentry-availability";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import type { SentryIssue } from "@/lib/types/sentry";
import { buildLinkedIssueTitle } from "./task-external-link-utils";
import { useTranslation } from "react-i18next";

export type ExternalLinkProvider = "jira" | "linear" | "sentry";

type TaskExternalLinkTarget = {
  id: string;
  title: string;
};

type TaskExternalLinkDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  provider: ExternalLinkProvider;
  task: TaskExternalLinkTarget;
  workspaceId: string;
};

/** Copy fields hold CATALOG KEYS, not English: `PROVIDERS` is a module-scope
 * table, so a resolved string would freeze at the boot locale (and
 * `i18next/no-literal-string` cannot see it). Each is resolved with `t()` at its
 * use site. */
type ProviderConfig = {
  titleKey: string;
  descriptionKey: string;
  inputLabelKey: string;
  placeholderKey: string;
  validationHintKey: string;
  successLabelKey: string;
  extractKey: (raw: string) => string | null;
  fetch: (key: string, workspaceId: string, instanceId?: string) => Promise<unknown>;
  resolveLinkedKey?: (requestedKey: string, result: unknown) => string;
  // requiresInstance gates providers (Sentry) whose fetch must target a chosen
  // instance within the workspace before it can run.
  requiresInstance?: boolean;
};

const SENTRY_NUMERIC_ISSUE_URL_RE = /\/issues\/(\d+)(?:[/?#]|$)/i;

function extractSentryIssueKey(raw: string): string | null {
  return extractSentryShortId(raw) ?? raw.match(SENTRY_NUMERIC_ISSUE_URL_RE)?.[1] ?? null;
}

function isSentryIssue(result: unknown): result is SentryIssue {
  return (
    typeof result === "object" &&
    result !== null &&
    typeof (result as Partial<SentryIssue>).shortId === "string"
  );
}

const PROVIDERS: Record<ExternalLinkProvider, ProviderConfig> = {
  jira: {
    titleKey: "task:linkJiraTicket",
    descriptionKey: "task:linkJiraTicketDescription",
    inputLabelKey: "task:ticket",
    placeholderKey: "task:jiraTicketPlaceholder",
    validationHintKey: "task:jiraTicketValidationHint",
    successLabelKey: "task:jiraTicketLinked",
    extractKey: (raw) => raw.toUpperCase().match(JIRA_KEY_RE)?.[0] ?? null,
    fetch: (key, workspaceId) => getJiraTicket(key, { workspaceId }),
  },
  linear: {
    titleKey: "task:linkLinearIssue",
    descriptionKey: "task:linkLinearIssueDescription",
    inputLabelKey: "task:issue",
    placeholderKey: "task:linearIssuePlaceholder",
    validationHintKey: "task:linearIssueValidationHint",
    successLabelKey: "task:linearIssueLinked",
    extractKey: (raw) => raw.toUpperCase().match(LINEAR_KEY_RE)?.[0] ?? null,
    fetch: (key, workspaceId) => getLinearIssue(key, { workspaceId }),
  },
  sentry: {
    titleKey: "task:linkSentryIssue",
    descriptionKey: "task:linkSentryIssueDescription",
    inputLabelKey: "task:issue",
    placeholderKey: "task:sentryIssuePlaceholder",
    validationHintKey: "task:sentryIssueValidationHint",
    successLabelKey: "task:sentryIssueLinked",
    extractKey: extractSentryIssueKey,
    fetch: (key, workspaceId, instanceId) => getSentryIssue(workspaceId, instanceId ?? "", key),
    resolveLinkedKey: (requestedKey, result) =>
      isSentryIssue(result) && result.shortId ? result.shortId : requestedKey,
    requiresInstance: true,
  },
};

// SentryLinkInstanceField resolves which Sentry instance a link targets: it
// auto-selects the sole healthy instance, prompts with a picker when several
// are healthy, and explains when none is usable. Owns the instances hook so the
// parent dialog stays provider-agnostic.
function SentryLinkInstanceField({
  workspaceId,
  instanceId,
  onChange,
}: {
  workspaceId: string;
  instanceId: string;
  onChange: (id: string) => void;
}) {
  const { t } = useTranslation();
  const sentry = useSentryInstances(workspaceId);

  // Depend on a by-value signature of the healthy instance IDs rather than the
  // `healthy` array identity: useSentryInstances rebuilds that array on every
  // poll (new reference, usually identical contents), which would otherwise
  // re-fire this effect needlessly. The signature still changes when the sole
  // healthy instance is swapped (state stays "single") or a selected instance
  // drops out of a "multi" set.
  const healthySignature = sentry.healthy
    .map((instance) => instance.id)
    .sort()
    .join("\n");

  useEffect(() => {
    const healthyIds = healthySignature ? healthySignature.split("\n") : [];
    if (sentry.state === "single") {
      onChange(healthyIds[0]);
    } else if (
      sentry.state === "empty" ||
      sentry.state === "unhealthy" ||
      (instanceId !== "" && !healthyIds.includes(instanceId))
    ) {
      onChange("");
    }
  }, [sentry.state, healthySignature, instanceId, onChange]);

  if (sentry.state === "empty" || sentry.state === "unhealthy") {
    return (
      <p className="text-xs text-muted-foreground" data-testid="sentry-link-no-instance">
        {t("task:connectAHealthySentryInstanceIn")}
      </p>
    );
  }
  if (sentry.state !== "multi") return null;
  return (
    <div className="space-y-2">
      <Label htmlFor="sentry-link-instance">{t("task:sentryInstance")}</Label>
      <Select value={instanceId} onValueChange={onChange}>
        <SelectTrigger id="sentry-link-instance" data-testid="sentry-link-instance-select">
          <SelectValue placeholder={t("task:selectAnInstance")} />
        </SelectTrigger>
        <SelectContent>
          {sentry.healthy.map((inst) => (
            <SelectItem key={inst.id} value={inst.id}>
              {inst.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

// useExternalLinkForm holds the dialog's transient state and the submit action
// (resolve key → fetch the external issue → rename the task). Extracted so the
// component body stays within the max-lines lint budget.
function useExternalLinkForm(
  config: ProviderConfig,
  task: TaskExternalLinkTarget,
  workspaceId: string,
  open: boolean,
  onOpenChange: (open: boolean) => void,
) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const store = useAppStoreApi();
  const [input, setInput] = useState("");
  const [instanceId, setInstanceId] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setInput("");
      setError(null);
      setInstanceId("");
    }
  }, [open]);

  const submit = async () => {
    const key = config.extractKey(input);
    if (!key) {
      setError(t(config.validationHintKey));
      return;
    }
    if (config.requiresInstance && !instanceId) {
      setError(t("task:selectSentryInstanceToLink"));
      return;
    }

    setSubmitting(true);
    setError(null);
    try {
      const result = await config.fetch(key, workspaceId, instanceId || undefined);
      const state = store.getState();
      const latestTask = findTaskInSnapshots(
        task.id,
        state.kanbanMulti.snapshots,
        state.kanban.tasks,
      );
      const linkedKey = config.resolveLinkedKey?.(key, result) ?? key;
      await updateTask(task.id, {
        title: buildLinkedIssueTitle(latestTask?.title ?? task.title, linkedKey),
      });
      toast({ description: t(config.successLabelKey), variant: "success" });
      onOpenChange(false);
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : t("task:failedToLinkTarget", { target: t(config.inputLabelKey) }),
      );
    } finally {
      setSubmitting(false);
    }
  };

  return { input, setInput, instanceId, setInstanceId, submitting, error, setError, submit };
}

export function TaskExternalLinkDialog({
  open,
  onOpenChange,
  provider,
  task,
  workspaceId,
}: TaskExternalLinkDialogProps) {
  const { t } = useTranslation();
  const config = PROVIDERS[provider];
  const { input, setInput, instanceId, setInstanceId, submitting, error, setError, submit } =
    useExternalLinkForm(config, task, workspaceId, open, onOpenChange);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[calc(100vw-2rem)] sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t(config.titleKey)}</DialogTitle>
          <DialogDescription>{t(config.descriptionKey)}</DialogDescription>
        </DialogHeader>
        {provider === "sentry" && (
          <SentryLinkInstanceField
            workspaceId={workspaceId}
            instanceId={instanceId}
            onChange={setInstanceId}
          />
        )}
        <div className="space-y-2">
          <Label htmlFor="task-external-link-input">{t(config.inputLabelKey)}</Label>
          <Input
            id="task-external-link-input"
            data-testid="task-external-link-input"
            value={input}
            onChange={(event) => {
              setInput(event.target.value);
              if (error) setError(null);
            }}
            placeholder={t(config.placeholderKey)}
            disabled={submitting}
          />
          {error && (
            <p className="text-xs text-destructive" data-testid="task-external-link-error">
              {error}
            </p>
          )}
        </div>
        <DialogFooter className="gap-2">
          <Button
            type="button"
            variant="outline"
            className="cursor-pointer"
            onClick={() => onOpenChange(false)}
            disabled={submitting}
          >
            {t("common:cancel")}
          </Button>
          <Button
            type="button"
            className="cursor-pointer"
            onClick={submit}
            disabled={submitting || !input.trim() || (config.requiresInstance && !instanceId)}
            data-dialog-default-action
            data-testid="task-external-link-submit"
          >
            {submitting ? t("task:saving") : t("common:save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
