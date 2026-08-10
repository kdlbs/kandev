"use client";

import { memo, useEffect, useMemo, useState } from "react";
import Link from "@/components/routing/app-link";
import {
  IconExternalLink,
  IconGitPullRequest,
  IconLink,
  IconLoader2,
  IconPlus,
  IconRefresh,
  IconUnlink,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useForgejoConfig } from "@/hooks/domains/forgejo/use-forgejo-config";
import { useForgejoTaskLinks } from "@/hooks/domains/forgejo/use-forgejo-task-links";
import { useTaskById } from "@/hooks/domains/kanban/use-task-by-id";
import {
  createForgejoTaskPullRequest,
  linkForgejoIssue,
  linkForgejoPullRequest,
} from "@/lib/api/domains/forgejo-api";
import type { Repository } from "@/lib/types/http";
import type { KanbanState } from "@/lib/state/slices";

type DialogMode = "create" | "link-pr" | "link-issue" | null;
type TaskLike = Pick<KanbanState["tasks"][number], "id" | "title" | "repositories">;

function taskRepositoryDefaults(task: TaskLike | null, repositories: Repository[] | undefined) {
  const linked = task?.repositories?.slice().sort((a, b) => a.position - b.position) ?? [];
  const taskRepository = linked[0];
  if (!taskRepository) return { repositoryID: "", owner: "", repo: "", head: "", base: "" };
  const repository = repositories?.find((item) => item.id === taskRepository.repository_id);
  return {
    repositoryID: taskRepository.repository_id,
    owner: repository?.provider_owner ?? "",
    repo: repository?.provider_name ?? "",
    // A valid task worktree is useful even before the Kandev repository has
    // been associated with a Forgejo provider identity. Users can fill owner/
    // repository, but should not need to rediscover the branch they pushed.
    head: taskRepository.checkout_branch || "",
    base: taskRepository.base_branch || repository?.default_branch || "",
  };
}

function ForgejoTaskDialog({
  mode,
  task,
  workspaceId,
  onOpenChange,
  onComplete,
}: {
  mode: DialogMode;
  task: TaskLike | null;
  workspaceId: string;
  onOpenChange: (open: boolean) => void;
  onComplete: () => Promise<void>;
}) {
  const repositories = useAppStore((state) => state.repositories.itemsByWorkspaceId[workspaceId]);
  const defaults = useMemo(() => taskRepositoryDefaults(task, repositories), [repositories, task]);
  const [owner, setOwner] = useState("");
  const [repo, setRepo] = useState("");
  const [head, setHead] = useState("");
  const [base, setBase] = useState("");
  const [number, setNumber] = useState("");
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const { toast } = useToast();

  useEffect(() => {
    if (!mode) return;
    setOwner(defaults.owner);
    setRepo(defaults.repo);
    setHead(defaults.head);
    setBase(defaults.base);
    setNumber("");
    setTitle(task?.title ?? "");
    setBody("");
  }, [defaults, mode, task?.title]);

  const submit = async () => {
    if (!task || !owner.trim() || !repo.trim() || submitting) return;
    setSubmitting(true);
    try {
      if (mode === "create") {
        if (!head.trim() || !base.trim() || !title.trim()) return;
        await createForgejoTaskPullRequest(
          {
            task_id: task.id,
            repository_id: defaults.repositoryID || undefined,
            owner: owner.trim(),
            repo: repo.trim(),
            title: title.trim(),
            body: body.trim() || undefined,
            head: head.trim(),
            base: base.trim(),
          },
          { workspaceId },
        );
      } else {
        const issueNumber = Number(number);
        if (!Number.isInteger(issueNumber) || issueNumber < 1) return;
        const payload = {
          task_id: task.id,
          repository_id: defaults.repositoryID || undefined,
          owner: owner.trim(),
          repo: repo.trim(),
          number: issueNumber,
        };
        if (mode === "link-issue") await linkForgejoIssue(payload, { workspaceId });
        else await linkForgejoPullRequest(payload, { workspaceId });
      }
      await onComplete();
      onOpenChange(false);
    } catch (cause) {
      toast({
        title:
          mode === "create"
            ? "Failed to create Forgejo pull request"
            : `Failed to link Forgejo ${mode === "link-issue" ? "issue" : "pull request"}`,
        description:
          cause instanceof Error ? cause.message : "Forgejo did not accept this request.",
        variant: "error",
      });
    } finally {
      setSubmitting(false);
    }
  };

  const canSubmit =
    !!owner.trim() &&
    !!repo.trim() &&
    (mode === "create" ? !!head.trim() && !!base.trim() && !!title.trim() : /^\d+$/.test(number));
  return (
    <Dialog open={mode !== null} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {mode === "create"
              ? "Create Forgejo pull request"
              : `Link Forgejo ${mode === "link-issue" ? "issue" : "pull request"}`}
          </DialogTitle>
          <DialogDescription>
            {mode === "create"
              ? "The source branch is prefilled from this task’s worktree. It must already be pushed to Forgejo."
              : `Associate an existing Forgejo ${mode === "link-issue" ? "issue" : "pull request"} with this task.`}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3 py-2">
          <div className="grid grid-cols-2 gap-3">
            <Field label="Owner" value={owner} onChange={setOwner} />
            <Field label="Repository" value={repo} onChange={setRepo} />
          </div>
          {mode === "create" ? (
            <>
              <Field label="Source branch" value={head} onChange={setHead} />
              <Field label="Base branch" value={base} onChange={setBase} />
              <Field label="Title" value={title} onChange={setTitle} />
              <div className="grid gap-1.5">
                <Label htmlFor="forgejo-pr-body">Description</Label>
                <textarea
                  id="forgejo-pr-body"
                  value={body}
                  onChange={(event) => setBody(event.target.value)}
                  className="min-h-20 rounded-md border bg-background px-3 py-2 text-sm"
                />
              </div>
            </>
          ) : (
            <Field
              label={`${mode === "link-issue" ? "Issue" : "Pull request"} number`}
              value={number}
              onChange={setNumber}
              inputMode="numeric"
            />
          )}
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            className="cursor-pointer"
            onClick={() => onOpenChange(false)}
          >
            Cancel
          </Button>
          <Button
            type="button"
            className="cursor-pointer"
            disabled={!canSubmit || submitting}
            onClick={() => void submit()}
          >
            {submitting
              ? "Saving…"
              : mode === "create"
                ? "Create pull request"
                : `Link ${mode === "link-issue" ? "issue" : "pull request"}`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Field({
  label,
  value,
  onChange,
  inputMode,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  inputMode?: "numeric";
}) {
  const id = `forgejo-${label.toLowerCase().replaceAll(" ", "-")}`;
  return (
    <div className="grid gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        inputMode={inputMode}
      />
    </div>
  );
}

export const ForgejoTaskLinksButton = memo(function ForgejoTaskLinksButton({
  compact = false,
  mobile = false,
}: {
  compact?: boolean;
  mobile?: boolean;
}) {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const task = useTaskById(taskId);
  const { config } = useForgejoConfig(workspaceId ?? undefined);
  const links = useForgejoTaskLinks(workspaceId, taskId);
  const [dialogMode, setDialogMode] = useState<DialogMode>(null);
  const { toast } = useToast();
  if (!workspaceId || !taskId || !config?.origin) return null;

  const action = (operation: () => Promise<void>, failure: string) =>
    void operation().catch((cause) =>
      toast({
        title: failure,
        description: cause instanceof Error ? cause.message : undefined,
        variant: "error",
      }),
    );
  const label =
    links.pullRequests.length === 1
      ? `Forgejo pull request #${links.pullRequests[0].pr_number}`
      : links.pullRequests.length
        ? `${links.pullRequests.length} Forgejo pull requests`
        : "Forgejo task links";
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            data-testid="forgejo-task-links-button"
            size={compact ? "icon-sm" : "sm"}
            variant="outline"
            className={
              mobile
                ? "h-11 w-11 cursor-pointer"
                : compact
                  ? "h-9 w-9 cursor-pointer"
                  : "cursor-pointer gap-1.5 px-2"
            }
            aria-label={label}
          >
            <IconGitPullRequest className="h-4 w-4 text-amber-500" />
            {!compact && (
              <span className="text-xs font-medium">
                {links.pullRequests.length ? `#${links.pullRequests[0].pr_number}` : "Forgejo"}
              </span>
            )}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-80">
          <DropdownMenuLabel>Forgejo links</DropdownMenuLabel>
          {links.loading ? (
            <DropdownMenuItem disabled>
              <IconLoader2 className="h-4 w-4 animate-spin" />
              Loading links…
            </DropdownMenuItem>
          ) : null}
          {links.error ? <DropdownMenuItem disabled>{links.error}</DropdownMenuItem> : null}
          {links.issues.map((issue) => (
            <div key={issue.id}>
              <DropdownMenuItem asChild className="cursor-pointer">
                <Link href={issue.issue_url} target="_blank" rel="noopener noreferrer">
                  <IconExternalLink className="h-4 w-4" />
                  Issue #{issue.issue_number}: {issue.title}
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem
                className="cursor-pointer"
                onSelect={() =>
                  action(() => links.refreshIssue(issue.id), "Failed to refresh Forgejo issue")
                }
              >
                <IconRefresh className="h-4 w-4" />
                Refresh issue
              </DropdownMenuItem>
              <DropdownMenuItem
                className="cursor-pointer text-destructive focus:text-destructive"
                onSelect={() =>
                  action(() => links.removeIssue(issue.id), "Failed to unlink Forgejo issue")
                }
              >
                <IconUnlink className="h-4 w-4" />
                Unlink issue
              </DropdownMenuItem>
            </div>
          ))}
          {links.pullRequests.map((pr) => (
            <div key={pr.id}>
              <DropdownMenuItem asChild className="cursor-pointer">
                <Link href={pr.pr_url} target="_blank" rel="noopener noreferrer">
                  <IconExternalLink className="h-4 w-4" />
                  Pull request #{pr.pr_number}: {pr.pr_title}
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem
                className="cursor-pointer"
                onSelect={() =>
                  action(
                    () => links.refreshPullRequest(pr.id),
                    "Failed to refresh Forgejo pull request",
                  )
                }
              >
                <IconRefresh className="h-4 w-4" />
                Refresh pull request
              </DropdownMenuItem>
              <DropdownMenuItem
                className="cursor-pointer text-destructive focus:text-destructive"
                onSelect={() =>
                  action(
                    () => links.removePullRequest(pr.id),
                    "Failed to unlink Forgejo pull request",
                  )
                }
              >
                <IconUnlink className="h-4 w-4" />
                Unlink pull request
              </DropdownMenuItem>
            </div>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem className="cursor-pointer" onSelect={() => setDialogMode("create")}>
            <IconPlus className="h-4 w-4" />
            Create pull request
          </DropdownMenuItem>
          <DropdownMenuItem className="cursor-pointer" onSelect={() => setDialogMode("link-pr")}>
            <IconLink className="h-4 w-4" />
            Link existing pull request
          </DropdownMenuItem>
          <DropdownMenuItem className="cursor-pointer" onSelect={() => setDialogMode("link-issue")}>
            <IconLink className="h-4 w-4" />
            Link existing issue
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <ForgejoTaskDialog
        mode={dialogMode}
        task={task}
        workspaceId={workspaceId}
        onOpenChange={(open) => !open && setDialogMode(null)}
        onComplete={links.reload}
      />
    </>
  );
});
