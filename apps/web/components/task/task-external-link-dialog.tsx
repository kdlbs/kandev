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
import { getJiraTicket } from "@/lib/api/domains/jira-api";
import { getLinearIssue } from "@/lib/api/domains/linear-api";
import { getSentryIssue } from "@/lib/api/domains/sentry-api";
import { updateTask } from "@/lib/api/domains/kanban-api";
import { JIRA_KEY_RE } from "@/components/jira/jira-ticket-common";
import { LINEAR_KEY_RE } from "@/components/linear/linear-issue-common";
import { SENTRY_SHORT_ID_RE } from "@/components/sentry/sentry-issue-common";
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
};

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
    extractKey: (raw) => {
      const upper = raw.toUpperCase().trim();
      return SENTRY_SHORT_ID_RE.test(upper)
        ? upper
        : (upper.match(/[A-Z][A-Z0-9_-]*-\d+/)?.[0] ?? null);
    },
    fetch: (key, workspaceId) => getSentryIssue(key, { workspaceId }),
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
      await config.fetch(key, workspaceId);
      await updateTask(task.id, { title: buildLinkedIssueTitle(task.title, key) });
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
            onChange={(event) => setInput(event.target.value)}
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
            data-testid="task-external-link-submit"
          >
            {submitting ? "Saving" : "Save"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
