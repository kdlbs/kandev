"use client";

import { useEffect, useState, type ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import Link from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";
import { useForgejoQueue } from "@/hooks/domains/forgejo/use-forgejo-queue";
import { useForgejoPullRequestDetails } from "@/hooks/domains/forgejo/use-forgejo-pull-request-details";
import { createTask } from "@/lib/api/domains/kanban-api";
import {
  linkForgejoIssue,
  linkForgejoPullRequest,
  listForgejoIssues,
  listForgejoRepositories,
} from "@/lib/api/domains/forgejo-api";
import type { ForgejoIssue, ForgejoRepository } from "@/lib/types/forgejo";

export function ForgejoPageClient({ workspaceId }: { workspaceId?: string }) {
  const { queue, loading, error, refresh } = useForgejoQueue(workspaceId);
  if (!workspaceId)
    return <ForgejoQueueMessage message="Choose a workspace to view its Forgejo queue." />;
  return (
    <ForgejoPage
      workspaceId={workspaceId}
      queue={queue}
      loading={loading}
      error={error}
      refresh={refresh}
    />
  );
}

function ForgejoPage({
  workspaceId,
  queue,
  loading,
  error,
  refresh,
}: {
  workspaceId: string;
  queue: ReturnType<typeof useForgejoQueue>["queue"];
  loading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
}) {
  const details = useForgejoPullRequestDetails(workspaceId);
  const workflowId = useAppStore((state) => state.workflows.activeId);
  const startStepId = useAppStore((state) =>
    workflowId
      ? (state.kanbanMulti.snapshots[workflowId]?.steps.find((step) => step.is_start_step)?.id ??
        state.kanbanMulti.snapshots[workflowId]?.steps[0]?.id)
      : null,
  );
  const [taskMessage, setTaskMessage] = useState<string | null>(null);
  const [creatingIssueKey, setCreatingIssueKey] = useState<string | null>(null);
  const [existingTaskID, setExistingTaskID] = useState("");
  const [linkingKey, setLinkingKey] = useState<string | null>(null);
  const createIssueTask = async (entry: NonNullable<typeof queue>["issues"][number]) => {
    if (!workflowId || !startStepId) {
      setTaskMessage("Select a workflow with a start step before creating a Forgejo task.");
      return;
    }
    const key = `${entry.repository.owner}/${entry.repository.name}#${entry.issue.number}`;
    setCreatingIssueKey(key);
    setTaskMessage(null);
    try {
      const task = await createTask({
        workspace_id: workspaceId,
        workflow_id: workflowId,
        workflow_step_id: startStepId,
        title: entry.issue.title,
        description: `Source Forgejo issue: ${entry.issue.html_url}\n\n${entry.issue.body ?? ""}`,
        priority: "medium",
        metadata: {
          forgejo_issue: {
            owner: entry.repository.owner,
            repo: entry.repository.name,
            number: entry.issue.number,
            url: entry.issue.html_url,
          },
        },
      });
      await linkForgejoIssue(
        {
          task_id: task.id,
          owner: entry.repository.owner,
          repo: entry.repository.name,
          number: entry.issue.number,
        },
        { workspaceId },
      );
      setTaskMessage(
        `Created Kandev task “${task.title}” from Forgejo issue #${entry.issue.number}.`,
      );
    } catch (cause) {
      setTaskMessage(
        cause instanceof Error
          ? cause.message
          : "Could not create a Kandev task from this Forgejo issue.",
      );
    } finally {
      setCreatingIssueKey(null);
    }
  };
  const linkExisting = async (
    kind: "issue" | "pull_request",
    owner: string,
    repo: string,
    number: number,
  ) => {
    const taskID = existingTaskID.trim();
    if (!taskID) return;
    const key = `${kind}:${owner}/${repo}#${number}`;
    setLinkingKey(key);
    setTaskMessage(null);
    try {
      if (kind === "issue")
        await linkForgejoIssue({ task_id: taskID, owner, repo, number }, { workspaceId });
      else await linkForgejoPullRequest({ task_id: taskID, owner, repo, number }, { workspaceId });
      setTaskMessage(
        `Linked Forgejo ${kind === "issue" ? "issue" : "pull request"} #${number} to Kandev task ${taskID}.`,
      );
    } catch (cause) {
      setTaskMessage(
        cause instanceof Error
          ? cause.message
          : "Could not link the Forgejo item to this Kandev task.",
      );
    } finally {
      setLinkingKey(null);
    }
  };
  return (
    <main className="mx-auto max-w-5xl space-y-6 p-4 sm:p-6">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Forgejo</h1>
          <p className="text-sm text-muted-foreground">
            Open issues and pull requests from your connected Forgejo account.
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            className="cursor-pointer"
            variant="outline"
            onClick={() => void refresh()}
            disabled={loading}
          >
            {loading ? "Refreshing…" : "Refresh"}
          </Button>
          <Button className="cursor-pointer" asChild>
            <Link href="/settings/integrations/forgejo">Connection settings</Link>
          </Button>
        </div>
      </header>
      {error ? <ForgejoQueueMessage message={error} /> : null}
      {taskMessage ? <ForgejoQueueMessage message={taskMessage} /> : null}
      <Card>
        <CardHeader>
          <CardTitle>Link to an existing Kandev task</CardTitle>
          <CardDescription>
            Enter the task ID, then choose Link existing task below.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Input
            aria-label="Existing Kandev task ID"
            placeholder="Kandev task ID"
            value={existingTaskID}
            onChange={(event) => setExistingTaskID(event.target.value)}
          />
        </CardContent>
      </Card>
      <ForgejoRepositoryIssueBrowser
        workspaceId={workspaceId}
        existingTaskID={existingTaskID}
        onCreateTask={(repository, issue) => createIssueTask({ repository, issue })}
        onLinkTask={(repository, issue) =>
          linkExisting("issue", repository.owner, repository.name, issue.number)
        }
      />
      {!error &&
      !loading &&
      queue &&
      queue.issues.length === 0 &&
      queue.pull_requests.length === 0 ? (
        <ForgejoQueueMessage message="No open Forgejo issues or pull requests were found." />
      ) : null}
      <QueueSection
        title="Open issues"
        empty="No open issues."
        items={queue?.issues ?? []}
        render={(entry) => {
          const key = `${entry.repository.owner}/${entry.repository.name}#${entry.issue.number}`;
          return (
            <span className="flex flex-wrap items-center gap-2">
              <a
                className="hover:underline"
                href={entry.issue.html_url}
                target="_blank"
                rel="noreferrer"
              >
                {entry.repository.full_name} #{entry.issue.number}: {entry.issue.title}
              </a>
              <Button
                className="cursor-pointer"
                size="sm"
                variant="outline"
                onClick={() => void createIssueTask(entry)}
                disabled={creatingIssueKey === key}
              >
                {creatingIssueKey === key ? "Creating…" : "Create Kandev task"}
              </Button>
              <Button
                className="cursor-pointer"
                size="sm"
                variant="outline"
                onClick={() =>
                  void linkExisting(
                    "issue",
                    entry.repository.owner,
                    entry.repository.name,
                    entry.issue.number,
                  )
                }
                disabled={!existingTaskID.trim() || linkingKey === `issue:${key}`}
              >
                {linkingKey === `issue:${key}` ? "Linking…" : "Link existing task"}
              </Button>
            </span>
          );
        }}
      />
      <QueueSection
        title="Open pull requests"
        empty="No open pull requests."
        items={queue?.pull_requests ?? []}
        render={(entry) => (
          <span className="flex flex-wrap items-center gap-2">
            <a
              className="hover:underline"
              href={entry.pull_request.html_url}
              target="_blank"
              rel="noreferrer"
            >
              {entry.repository.full_name} #{entry.pull_request.number}: {entry.pull_request.title}
            </a>
            <Button
              className="cursor-pointer"
              size="sm"
              variant="outline"
              onClick={() =>
                void details.load(
                  entry.repository.owner,
                  entry.repository.name,
                  entry.pull_request.number,
                )
              }
            >
              Details
            </Button>
            <Button
              className="cursor-pointer"
              size="sm"
              variant="outline"
              onClick={() =>
                void linkExisting(
                  "pull_request",
                  entry.repository.owner,
                  entry.repository.name,
                  entry.pull_request.number,
                )
              }
              disabled={
                !existingTaskID.trim() ||
                linkingKey ===
                  `pull_request:${entry.repository.owner}/${entry.repository.name}#${entry.pull_request.number}`
              }
            >
              {linkingKey ===
              `pull_request:${entry.repository.owner}/${entry.repository.name}#${entry.pull_request.number}`
                ? "Linking…"
                : "Link existing task"}
            </Button>
          </span>
        )}
      />
      {details.loading ? <ForgejoQueueMessage message="Loading pull request details…" /> : null}
      {details.error ? <ForgejoQueueMessage message={details.error} /> : null}
      {details.details ? (
        <ForgejoPullRequestPanel
          details={details.details}
          comment={details.comment}
          review={details.review}
        />
      ) : null}
    </main>
  );
}

function ForgejoRepositoryIssueBrowser({
  workspaceId,
  existingTaskID,
  onCreateTask,
  onLinkTask,
}: {
  workspaceId: string;
  existingTaskID: string;
  onCreateTask: (repository: ForgejoRepository, issue: ForgejoIssue) => Promise<void>;
  onLinkTask: (repository: ForgejoRepository, issue: ForgejoIssue) => Promise<void>;
}) {
  const [repositories, setRepositories] = useState<ForgejoRepository[]>([]);
  const [selected, setSelected] = useState("");
  const [issues, setIssues] = useState<ForgejoIssue[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [error, setError] = useState<string | null>(null);
  const limit = 30;
  const repository = repositories.find((item) => item.full_name === selected);

  useEffect(() => {
    let active = true;
    void listForgejoRepositories({ workspaceId, limit: 30 })
      .then((result) => {
        if (!active) return;
        setRepositories(result.repositories);
        setSelected((current) => current || result.repositories[0]?.full_name || "");
      })
      .catch(
        (cause) =>
          active &&
          setError(cause instanceof Error ? cause.message : "Could not load Forgejo repositories"),
      );
    return () => {
      active = false;
    };
  }, [workspaceId]);

  useEffect(() => {
    if (!repository) return;
    let active = true;
    setError(null);
    void listForgejoIssues(repository.owner, repository.name, { workspaceId, page, limit })
      .then((result) => {
        if (!active) return;
        setIssues(result.issues);
        setTotal(result.total_count);
      })
      .catch(
        (cause) =>
          active &&
          setError(cause instanceof Error ? cause.message : "Could not load Forgejo issues"),
      );
    return () => {
      active = false;
    };
  }, [page, repository, workspaceId]);

  const pages = Math.max(1, Math.ceil(total / limit));
  return (
    <Card>
      <CardHeader>
        <CardTitle>Browse repository issues</CardTitle>
        <CardDescription>
          Select an accessible repository to browse open Forgejo issues by page.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <select
          aria-label="Forgejo repository"
          className="h-9 w-full rounded-md border bg-background px-3 text-sm"
          value={selected}
          onChange={(event) => {
            setSelected(event.target.value);
            setPage(1);
          }}
        >
          <option value="">Select a repository</option>
          {repositories.map((item) => (
            <option key={item.full_name} value={item.full_name}>
              {item.full_name}
            </option>
          ))}
        </select>
        {error ? (
          <p role="status" className="text-sm text-destructive">
            {error}
          </p>
        ) : null}
        {repository && !error ? (
          <ul className="space-y-2 text-sm">
            {issues.map((issue) => (
              <li className="flex flex-wrap items-center gap-2" key={issue.number}>
                <a
                  className="hover:underline"
                  href={issue.html_url}
                  target="_blank"
                  rel="noreferrer"
                >
                  #{issue.number}: {issue.title}
                </a>
                <Button
                  className="cursor-pointer"
                  size="sm"
                  variant="outline"
                  onClick={() => void onCreateTask(repository, issue)}
                >
                  Create Kandev task
                </Button>
                <Button
                  className="cursor-pointer"
                  size="sm"
                  variant="outline"
                  disabled={!existingTaskID.trim()}
                  onClick={() => void onLinkTask(repository, issue)}
                >
                  Link existing task
                </Button>
              </li>
            ))}
          </ul>
        ) : null}
        {repository ? (
          <div className="flex items-center justify-between gap-2 text-sm">
            <span>
              Page {page} of {pages}
            </span>
            <div className="flex gap-2">
              <Button
                className="cursor-pointer"
                size="sm"
                variant="outline"
                disabled={page === 1}
                onClick={() => setPage((current) => current - 1)}
              >
                Previous
              </Button>
              <Button
                className="cursor-pointer"
                size="sm"
                variant="outline"
                disabled={page >= pages}
                onClick={() => setPage((current) => current + 1)}
              >
                Next
              </Button>
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

function ForgejoQueueMessage({ message }: { message: string }) {
  return (
    <Card>
      <CardContent className="p-5 text-sm text-muted-foreground">{message}</CardContent>
    </Card>
  );
}
function QueueSection<T>({
  title,
  empty,
  items,
  render,
}: {
  title: string;
  empty: string;
  items: T[];
  render: (item: T) => ReactNode;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>
          Links open in Forgejo; use a task’s link panel to associate a specific item.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {items.length ? (
          <ul className="space-y-2 text-sm">
            {items.map((item, index) => (
              <li key={index}>{render(item)}</li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">{empty}</p>
        )}
      </CardContent>
    </Card>
  );
}
function ForgejoPullRequestPanel({
  details,
  comment,
  review,
}: {
  details: import("@/lib/types/forgejo").ForgejoPullRequestDetails;
  comment: (owner: string, repo: string, number: number, body: string) => Promise<void>;
  review: (
    owner: string,
    repo: string,
    number: number,
    event: "APPROVE" | "REQUEST_CHANGES" | "COMMENT",
    body?: string,
  ) => Promise<void>;
}) {
  const [body, setBody] = useState("");
  const pull = details.pull_request;
  const submit = (event: "APPROVE" | "REQUEST_CHANGES" | "COMMENT") => {
    void review(details.owner, details.repo, pull.number, event, body);
  };
  return (
    <Card>
      <CardHeader>
        <CardTitle>{pull.title}</CardTitle>
        <CardDescription>
          {pull.mergeable ? "Mergeable" : "Not currently mergeable"} · {details.files.length}{" "}
          changed files · {details.commits.length} commits · {details.action_runs.length} Actions
          runs
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <textarea
          className="min-h-20 w-full rounded border p-2"
          placeholder="Comment or review summary"
          value={body}
          onChange={(event) => setBody(event.target.value)}
        />
        <div className="flex flex-wrap gap-2">
          <Button
            className="cursor-pointer"
            size="sm"
            onClick={() => void comment(details.owner, details.repo, pull.number, body)}
            disabled={!body}
          >
            Comment
          </Button>
          <Button
            className="cursor-pointer"
            size="sm"
            variant="outline"
            onClick={() => submit("APPROVE")}
          >
            Approve
          </Button>
          <Button
            className="cursor-pointer"
            size="sm"
            variant="outline"
            onClick={() => submit("REQUEST_CHANGES")}
          >
            Request changes
          </Button>
        </div>
        <div>
          <p className="font-medium">Reviews</p>
          {details.reviews.length ? (
            <ul>
              {details.reviews.map((item) => (
                <li key={item.id}>
                  {item.reviewer}: {item.state}
                  {item.body ? ` — ${item.body}` : ""}
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-muted-foreground">No reviews yet.</p>
          )}
        </div>
        <div>
          <p className="font-medium">Comments</p>
          {details.comments.length ? (
            <ul>
              {details.comments.map((item) => (
                <li key={item.id}>
                  {item.author}: {item.body}
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-muted-foreground">No comments yet.</p>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
