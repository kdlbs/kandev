"use client";

import type { ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import Link from "@/components/routing/app-link";
import { useForgejoQueue } from "@/hooks/domains/forgejo/use-forgejo-queue";

export function ForgejoPageClient({ workspaceId }: { workspaceId?: string }) {
  const { queue, loading, error, refresh } = useForgejoQueue(workspaceId);
  if (!workspaceId) return <ForgejoQueueMessage message="Choose a workspace to view its Forgejo queue." />;
  return <main className="mx-auto max-w-5xl space-y-6 p-4 sm:p-6">
    <header className="flex flex-wrap items-center justify-between gap-3"><div><h1 className="text-2xl font-semibold">Forgejo</h1><p className="text-sm text-muted-foreground">Open issues and pull requests from your connected Forgejo account.</p></div><div className="flex gap-2"><Button className="cursor-pointer" variant="outline" onClick={() => void refresh()} disabled={loading}>{loading ? "Refreshing…" : "Refresh"}</Button><Button className="cursor-pointer" asChild><Link href="/settings/integrations/forgejo">Connection settings</Link></Button></div></header>
    {error ? <ForgejoQueueMessage message={error} /> : null}
    {!error && !loading && queue && queue.issues.length === 0 && queue.pull_requests.length === 0 ? <ForgejoQueueMessage message="No open Forgejo issues or pull requests were found." /> : null}
    <QueueSection title="Open issues" empty="No open issues." items={queue?.issues ?? []} render={(entry) => <a className="hover:underline" href={entry.issue.html_url} target="_blank" rel="noreferrer">{entry.repository.full_name} #{entry.issue.number}: {entry.issue.title}</a>} />
    <QueueSection title="Open pull requests" empty="No open pull requests." items={queue?.pull_requests ?? []} render={(entry) => <a className="hover:underline" href={entry.pull_request.html_url} target="_blank" rel="noreferrer">{entry.repository.full_name} #{entry.pull_request.number}: {entry.pull_request.title}</a>} />
  </main>;
}

function ForgejoQueueMessage({ message }: { message: string }) { return <Card><CardContent className="p-5 text-sm text-muted-foreground">{message}</CardContent></Card>; }
function QueueSection<T>({ title, empty, items, render }: { title: string; empty: string; items: T[]; render: (item: T) => ReactNode }) { return <Card><CardHeader><CardTitle>{title}</CardTitle><CardDescription>Links open in Forgejo; use a task’s link panel to associate a specific item.</CardDescription></CardHeader><CardContent>{items.length ? <ul className="space-y-2 text-sm">{items.map((item, index) => <li key={index}>{render(item)}</li>)}</ul> : <p className="text-sm text-muted-foreground">{empty}</p>}</CardContent></Card>; }
