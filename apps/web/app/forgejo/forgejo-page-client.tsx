"use client";

import { useState, type ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import Link from "@/components/routing/app-link";
import { useForgejoQueue } from "@/hooks/domains/forgejo/use-forgejo-queue";
import { useForgejoPullRequestDetails } from "@/hooks/domains/forgejo/use-forgejo-pull-request-details";

export function ForgejoPageClient({ workspaceId }: { workspaceId?: string }) {
  const { queue, loading, error, refresh } = useForgejoQueue(workspaceId);
  if (!workspaceId) return <ForgejoQueueMessage message="Choose a workspace to view its Forgejo queue." />;
  return <ForgejoPage workspaceId={workspaceId} queue={queue} loading={loading} error={error} refresh={refresh} />;
}

function ForgejoPage({ workspaceId, queue, loading, error, refresh }: { workspaceId: string; queue: ReturnType<typeof useForgejoQueue>["queue"]; loading: boolean; error: string | null; refresh: () => Promise<void> }) {
  const details = useForgejoPullRequestDetails(workspaceId);
  return <main className="mx-auto max-w-5xl space-y-6 p-4 sm:p-6">
    <header className="flex flex-wrap items-center justify-between gap-3"><div><h1 className="text-2xl font-semibold">Forgejo</h1><p className="text-sm text-muted-foreground">Open issues and pull requests from your connected Forgejo account.</p></div><div className="flex gap-2"><Button className="cursor-pointer" variant="outline" onClick={() => void refresh()} disabled={loading}>{loading ? "Refreshing…" : "Refresh"}</Button><Button className="cursor-pointer" asChild><Link href="/settings/integrations/forgejo">Connection settings</Link></Button></div></header>
    {error ? <ForgejoQueueMessage message={error} /> : null}
    {!error && !loading && queue && queue.issues.length === 0 && queue.pull_requests.length === 0 ? <ForgejoQueueMessage message="No open Forgejo issues or pull requests were found." /> : null}
    <QueueSection title="Open issues" empty="No open issues." items={queue?.issues ?? []} render={(entry) => <a className="hover:underline" href={entry.issue.html_url} target="_blank" rel="noreferrer">{entry.repository.full_name} #{entry.issue.number}: {entry.issue.title}</a>} />
    <QueueSection title="Open pull requests" empty="No open pull requests." items={queue?.pull_requests ?? []} render={(entry) => <span className="flex flex-wrap items-center gap-2"><a className="hover:underline" href={entry.pull_request.html_url} target="_blank" rel="noreferrer">{entry.repository.full_name} #{entry.pull_request.number}: {entry.pull_request.title}</a><Button className="cursor-pointer" size="sm" variant="outline" onClick={() => void details.load(entry.repository.owner, entry.repository.name, entry.pull_request.number)}>Details</Button></span>} />
    {details.loading ? <ForgejoQueueMessage message="Loading pull request details…" /> : null}
    {details.error ? <ForgejoQueueMessage message={details.error} /> : null}
    {details.details ? <ForgejoPullRequestPanel details={details.details} comment={details.comment} review={details.review} /> : null}
  </main>;
}

function ForgejoQueueMessage({ message }: { message: string }) { return <Card><CardContent className="p-5 text-sm text-muted-foreground">{message}</CardContent></Card>; }
function QueueSection<T>({ title, empty, items, render }: { title: string; empty: string; items: T[]; render: (item: T) => ReactNode }) { return <Card><CardHeader><CardTitle>{title}</CardTitle><CardDescription>Links open in Forgejo; use a task’s link panel to associate a specific item.</CardDescription></CardHeader><CardContent>{items.length ? <ul className="space-y-2 text-sm">{items.map((item, index) => <li key={index}>{render(item)}</li>)}</ul> : <p className="text-sm text-muted-foreground">{empty}</p>}</CardContent></Card>; }
function ForgejoPullRequestPanel({ details, comment, review }: { details: import("@/lib/types/forgejo").ForgejoPullRequestDetails; comment: (owner: string, repo: string, number: number, body: string) => Promise<void>; review: (owner: string, repo: string, number: number, event: "APPROVE" | "REQUEST_CHANGES" | "COMMENT", body?: string) => Promise<void> }) { const [body, setBody] = useState(""); const pull = details.pull_request; const submit = (event: "APPROVE" | "REQUEST_CHANGES" | "COMMENT") => { void review(details.owner, details.repo, pull.number, event, body); }; return <Card><CardHeader><CardTitle>{pull.title}</CardTitle><CardDescription>{pull.mergeable ? "Mergeable" : "Not currently mergeable"} · {details.files.length} changed files · {details.commits.length} commits · {details.action_runs.length} Actions runs</CardDescription></CardHeader><CardContent className="space-y-3 text-sm"><textarea className="min-h-20 w-full rounded border p-2" placeholder="Comment or review summary" value={body} onChange={(event) => setBody(event.target.value)} /><div className="flex flex-wrap gap-2"><Button className="cursor-pointer" size="sm" onClick={() => void comment(details.owner, details.repo, pull.number, body)} disabled={!body}>Comment</Button><Button className="cursor-pointer" size="sm" variant="outline" onClick={() => submit("APPROVE")}>Approve</Button><Button className="cursor-pointer" size="sm" variant="outline" onClick={() => submit("REQUEST_CHANGES")}>Request changes</Button></div><div><p className="font-medium">Reviews</p>{details.reviews.length ? <ul>{details.reviews.map((item) => <li key={item.id}>{item.reviewer}: {item.state}{item.body ? ` — ${item.body}` : ""}</li>)}</ul> : <p className="text-muted-foreground">No reviews yet.</p>}</div><div><p className="font-medium">Comments</p>{details.comments.length ? <ul>{details.comments.map((item) => <li key={item.id}>{item.author}: {item.body}</li>)}</ul> : <p className="text-muted-foreground">No comments yet.</p>}</div></CardContent></Card>; }
