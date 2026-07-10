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
import { useToast } from "@/components/toast-provider";
import { useAppStoreApi } from "@/components/state-provider";
import { getJiraTicket } from "@/lib/api/domains/jira-api";
import { getLinearIssue } from "@/lib/api/domains/linear-api";
import { getSentryIssue } from "@/lib/api/domains/sentry-api";
import { updateTask } from "@/lib/api/domains/kanban-api";
import { JIRA_KEY_RE } from "@/components/jira/jira-ticket-common";
import { LINEAR_KEY_RE } from "@/components/linear/linear-issue-common";
import { extractSentryShortId } from "@/components/sentry/sentry-issue-common";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
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
  fetch: (key: string, workspaceId: string) => Promise<unknown>;
  resolveLinkedKey?: (requestedKey: string, result: unknown) => string;
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
    fetch: (key, workspaceId) => getSentryIssue(key, { workspaceId }),
    resolveLinkedKey: (requestedKey, result) =>
      isSentryIssue(result) && result.shortId ? result.shortId : requestedKey,
  },
};

export function TaskExternalLinkDialog({
  open,
  onOpenChange,
  provider,
  task,
  workspaceId,
}: TaskExternalLinkDialogProps) {
  const { toast } = useToast();
  const store = useAppStoreApi();
  const config = PROVIDERS[provider];
  const [input, setInput] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setInput("");
      setError(null);
    }
  }, [open]);

  const submit = async () => {
    const key = config.extractKey(input);
    if (!key) {
      setError(config.validationHint);
      return;
    }

    setSubmitting(true);
    setError(null);
    try {
      const result = await config.fetch(key, workspaceId);
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
      toast({ description: config.successLabel, variant: "success" });
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : `Failed to link ${config.inputLabel}.`);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[calc(100vw-2rem)] sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{config.title}</DialogTitle>
          <DialogDescription>{config.description}</DialogDescription>
        </DialogHeader>
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
            disabled={submitting || !input.trim()}
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
