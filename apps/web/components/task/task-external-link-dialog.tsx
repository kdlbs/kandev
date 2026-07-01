"use client";

import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
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
import { getJiraTicket } from "@/lib/api/domains/jira-api";
import { getLinearIssue } from "@/lib/api/domains/linear-api";
import { getSentryIssue } from "@/lib/api/domains/sentry-api";
import { updateTask } from "@/lib/api/domains/kanban-api";
import { JIRA_KEY_RE } from "@/components/jira/jira-ticket-common";
import { LINEAR_KEY_RE } from "@/components/linear/linear-issue-common";
import { extractSentryShortId } from "@/components/sentry/sentry-issue-common";
import { useSentryInstances } from "@/hooks/domains/sentry/use-sentry-availability";
import { workflowSnapshotQueryData } from "@/lib/query/workflow-snapshot-cache";
import type { SentryIssue } from "@/lib/types/sentry";
import { buildLinkedIssueTitle } from "./task-external-link-utils";

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

type ProviderConfig = {
  title: string;
  description: string;
  inputLabel: string;
  placeholder: string;
  validationHint: string;
  successLabel: string;
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
    title: "Link Jira ticket",
    description: "Use a Jira ticket key or URL for this task.",
    inputLabel: "Ticket",
    placeholder: "PROJ-123 or paste ticket URL",
    validationHint: "Paste a Jira ticket URL or key (PROJ-123).",
    successLabel: "Jira ticket linked",
    extractKey: (raw) => raw.toUpperCase().match(JIRA_KEY_RE)?.[0] ?? null,
    fetch: (key, workspaceId) => getJiraTicket(key, { workspaceId }),
  },
  linear: {
    title: "Link Linear issue",
    description: "Use a Linear issue identifier or URL for this task.",
    inputLabel: "Issue",
    placeholder: "ENG-123 or paste issue URL",
    validationHint: "Paste a Linear issue URL or identifier (ENG-123).",
    successLabel: "Linear issue linked",
    extractKey: (raw) => raw.toUpperCase().match(LINEAR_KEY_RE)?.[0] ?? null,
    fetch: (key, workspaceId) => getLinearIssue(key, { workspaceId }),
  },
  sentry: {
    title: "Link Sentry issue",
    description: "Use a Sentry short ID or URL for this task.",
    inputLabel: "Issue",
    placeholder: "PROJ-123 or paste issue URL",
    validationHint: "Paste a Sentry issue URL or short ID (PROJ-123).",
    successLabel: "Sentry issue linked",
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
        Connect a healthy Sentry instance in Settings → Integrations → Sentry to link issues.
      </p>
    );
  }
  if (sentry.state !== "multi") return null;
  return (
    <div className="space-y-2">
      <Label htmlFor="sentry-link-instance">Sentry instance</Label>
      <Select value={instanceId} onValueChange={onChange}>
        <SelectTrigger id="sentry-link-instance" data-testid="sentry-link-instance-select">
          <SelectValue placeholder="Select an instance" />
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
  const { toast } = useToast();
  const queryClient = useQueryClient();
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
      setError(config.validationHint);
      return;
    }
    if (config.requiresInstance && !instanceId) {
      setError("Select a Sentry instance to link against.");
      return;
    }

    setSubmitting(true);
    setError(null);
    try {
      const result = await config.fetch(key, workspaceId, instanceId || undefined);
      const latestTask = workflowSnapshotQueryData(queryClient)
        .flatMap((snapshot) => snapshot.tasks)
        .find((snapshotTask) => snapshotTask.id === task.id);
      const linkedKey = config.resolveLinkedKey?.(key, result) ?? key;
      await updateTask(task.id, {
        title: buildLinkedIssueTitle(latestTask?.title ?? task.title, linkedKey),
      });
      toast({ description: config.successLabel, variant: "success" });
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : `Failed to link ${config.inputLabel}.`);
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
  const config = PROVIDERS[provider];
  const { input, setInput, instanceId, setInstanceId, submitting, error, setError, submit } =
    useExternalLinkForm(config, task, workspaceId, open, onOpenChange);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[calc(100vw-2rem)] sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{config.title}</DialogTitle>
          <DialogDescription>{config.description}</DialogDescription>
        </DialogHeader>
        {provider === "sentry" && (
          <SentryLinkInstanceField
            workspaceId={workspaceId}
            instanceId={instanceId}
            onChange={setInstanceId}
          />
        )}
        <div className="space-y-2">
          <Label htmlFor="task-external-link-input">{config.inputLabel}</Label>
          <Input
            id="task-external-link-input"
            data-testid="task-external-link-input"
            value={input}
            onChange={(event) => {
              setInput(event.target.value);
              if (error) setError(null);
            }}
            placeholder={config.placeholder}
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
            Cancel
          </Button>
          <Button
            type="button"
            className="cursor-pointer"
            onClick={submit}
            disabled={submitting || !input.trim() || (config.requiresInstance && !instanceId)}
            data-dialog-default-action
            data-testid="task-external-link-submit"
          >
            {submitting ? "Saving" : "Save"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
