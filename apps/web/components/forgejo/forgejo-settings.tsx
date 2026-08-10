"use client";

import { useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
import { SettingsCard } from "@/components/settings/settings-card";
import { deleteForgejoConfig, deleteForgejoIssueWatch, getForgejoConfig, listForgejoIssueWatches, listForgejoQueue, listForgejoRepositories, pollForgejoIssueWatch, refreshForgejoConnection, saveForgejoIssueWatch, setForgejoConfig, testForgejoConfig } from "@/lib/api/domains/forgejo-api";
import type { ForgejoConfig, ForgejoIssueWatch, ForgejoRepository } from "@/lib/types/forgejo";

function ForgejoConnection({ workspaceId }: { workspaceId: string }) {
  const [config, setConfig] = useState<ForgejoConfig | null>(null);
  const [origin, setOrigin] = useState("");
  const [token, setToken] = useState("");
  const [repositories, setRepositories] = useState<ForgejoRepository[]>([]);
	const [queue, setQueue] = useState<{ issues: { repository: ForgejoRepository; issue: { number: number; title: string } }[]; pull_requests: { repository: ForgejoRepository; pull_request: { number: number; title: string } }[] } | null>(null);
	const [watches, setWatches] = useState<ForgejoIssueWatch[]>([]);
	const [watchOwner, setWatchOwner] = useState("");
	const [watchRepo, setWatchRepo] = useState("");
	const [watchLabels, setWatchLabels] = useState("");
	const [watchWorkflow, setWatchWorkflow] = useState("");
	const [watchStep, setWatchStep] = useState("");
	const [watchRepository, setWatchRepository] = useState("");
	const [watchBaseBranch, setWatchBaseBranch] = useState("");
	const [watchPrompt, setWatchPrompt] = useState("");
	const [watchAgentProfile, setWatchAgentProfile] = useState("");
  const [message, setMessage] = useState("");

  useEffect(() => { void getForgejoConfig({ workspaceId }).then((value) => { setConfig(value); setOrigin(value?.origin ?? ""); }); }, [workspaceId]);
  const options = { workspaceId };
  const test = async () => { const result = await testForgejoConfig({ origin, token }, options); setMessage(result.ok ? `Connected as ${result.username}` : result.error ?? "Connection failed"); };
  const save = async () => { const next = await setForgejoConfig({ origin, token: token || undefined }, options); setConfig(next); setToken(""); setMessage(`Saved connection for ${next.username}`); };
  const loadRepositories = async () => { const result = await listForgejoRepositories(options); setRepositories(result.repositories); };
  const loadQueue = async () => { setQueue(await listForgejoQueue(options)); };
	const refresh = async () => { const next = await refreshForgejoConnection(options); setConfig(next); setMessage(next.last_ok ? "Connection is healthy" : next.last_error || "Connection check failed"); };
	const disconnect = async () => { await deleteForgejoConfig(options); setConfig(null); setToken(""); setRepositories([]); setQueue(null); setMessage("Forgejo disconnected from this workspace"); };
	const loadWatches = async () => { const result = await listForgejoIssueWatches(options); setWatches(result.watches); };
	const saveWatch = async () => { await saveForgejoIssueWatch({ owner: watchOwner, repo: watchRepo, labels: watchLabels, workflow_id: watchWorkflow, workflow_step_id: watchStep, repository_id: watchRepository, base_branch: watchBaseBranch, prompt: watchPrompt, agent_profile_id: watchAgentProfile, enabled: true }, options); await loadWatches(); setWatchOwner(""); setWatchRepo(""); setWatchLabels(""); setWatchWorkflow(""); setWatchStep(""); setWatchRepository(""); setWatchBaseBranch(""); setWatchPrompt(""); setWatchAgentProfile(""); };
	const pollWatch = async (watch: ForgejoIssueWatch) => { const result = await pollForgejoIssueWatch(watch.id, options); setMessage(`Watch found ${result.issues.length} matching issue${result.issues.length === 1 ? "" : "s"}`); await loadWatches(); };
	const deleteWatch = async (watch: ForgejoIssueWatch) => { await deleteForgejoIssueWatch(watch.id, options); await loadWatches(); };

  return <SettingsCard>
    <CardHeader><CardTitle>Forgejo</CardTitle><CardDescription>Connect this workspace to a Forgejo server. The token is stored in Kandev’s secret store and is never made available to agents.</CardDescription></CardHeader>
    <CardContent className="space-y-3">
      <Input data-testid="forgejo-origin-input" type="url" placeholder="https://forgejo.example" value={origin} onChange={(event) => setOrigin(event.target.value)} />
      <Input data-testid="forgejo-token-input" type="password" placeholder={config?.has_secret ? "Token saved — enter a new value to replace it" : "Forgejo personal access token"} value={token} onChange={(event) => setToken(event.target.value)} />
      <div className="flex flex-wrap gap-2"><Button className="cursor-pointer" type="button" onClick={() => void test()}>Test connection</Button><Button className="cursor-pointer" type="button" onClick={() => void save()}>Save connection</Button>{config?.has_secret ? <Button className="cursor-pointer" variant="outline" type="button" onClick={() => void refresh()}>Refresh connection</Button> : null}{config?.has_secret ? <Button className="cursor-pointer" variant="outline" type="button" onClick={() => void loadRepositories()}>Load repositories</Button> : null}{config?.has_secret ? <Button className="cursor-pointer" variant="outline" type="button" onClick={() => void loadQueue()}>Refresh queue</Button> : null}{config?.has_secret ? <Button className="cursor-pointer" variant="destructive" type="button" onClick={() => void disconnect()}>Disconnect</Button> : null}</div>
      {message ? <p role="status" className="text-sm text-muted-foreground">{message}</p> : null}
      {repositories.length ? <ul className="text-sm space-y-1">{repositories.map((repository) => <li key={repository.full_name}>{repository.full_name}</li>)}</ul> : null}
		{queue ? <div className="space-y-3 text-sm"><div><p className="font-medium">Open issues</p><ul className="space-y-1">{queue.issues.map(({ repository, issue }) => <li key={`${repository.full_name}-${issue.number}`}>{repository.full_name} #{issue.number}: {issue.title}</li>)}</ul></div><div><p className="font-medium">Open pull requests</p><ul className="space-y-1">{queue.pull_requests.map(({ repository, pull_request: pull }) => <li key={`${repository.full_name}-${pull.number}`}>{repository.full_name} #{pull.number}: {pull.title}</li>)}</ul></div></div> : null}
		{config?.has_secret ? <div className="space-y-2 border-t pt-3"><p className="font-medium text-sm">Issue watches</p><p className="text-sm text-muted-foreground">A matching issue creates a task in the workflow and step you choose. Use repository ID and agent profile ID when this workspace has multiple repositories or agents.</p><div className="grid gap-2 sm:grid-cols-3"><Input placeholder="Owner" value={watchOwner} onChange={(event) => setWatchOwner(event.target.value)} /><Input placeholder="Repository" value={watchRepo} onChange={(event) => setWatchRepo(event.target.value)} /><Input placeholder="Labels, optional" value={watchLabels} onChange={(event) => setWatchLabels(event.target.value)} /><Input placeholder="Workflow ID" value={watchWorkflow} onChange={(event) => setWatchWorkflow(event.target.value)} /><Input placeholder="Workflow step ID, optional" value={watchStep} onChange={(event) => setWatchStep(event.target.value)} /><Input placeholder="Kandev repository ID, optional" value={watchRepository} onChange={(event) => setWatchRepository(event.target.value)} /><Input placeholder="Base branch, optional" value={watchBaseBranch} onChange={(event) => setWatchBaseBranch(event.target.value)} /><Input placeholder="Agent profile ID, optional" value={watchAgentProfile} onChange={(event) => setWatchAgentProfile(event.target.value)} /><Input placeholder="Task instructions, optional" value={watchPrompt} onChange={(event) => setWatchPrompt(event.target.value)} /></div><div className="flex flex-wrap gap-2"><Button className="cursor-pointer" type="button" disabled={!watchOwner || !watchRepo || !watchWorkflow} onClick={() => void saveWatch()}>Save watch</Button><Button className="cursor-pointer" variant="outline" type="button" onClick={() => void loadWatches()}>Load watches</Button></div><ul className="space-y-1 text-sm">{watches.map((watch) => <li className="flex flex-wrap items-center gap-2" key={watch.id}><span>{watch.owner}/{watch.repo}{watch.labels ? ` · ${watch.labels}` : ""}{watch.workflow_id ? ` → ${watch.workflow_id}` : ""}</span><Button className="cursor-pointer" size="sm" variant="outline" type="button" onClick={() => void pollWatch(watch)}>Poll</Button><Button className="cursor-pointer" size="sm" variant="destructive" type="button" onClick={() => void deleteWatch(watch)}>Delete</Button></li>)}</ul></div> : null}
    </CardContent>
  </SettingsCard>;
}

export function ForgejoIntegrationPage({ workspaceId }: { workspaceId?: string } = {}) {
  return <WorkspaceScopedSection workspaceId={workspaceId}>{(ws) => <ForgejoConnection workspaceId={ws} />}</WorkspaceScopedSection>;
}
