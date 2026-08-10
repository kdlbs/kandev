"use client";

import { useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
import { SettingsCard } from "@/components/settings/settings-card";
import { getForgejoConfig, listForgejoRepositories, setForgejoConfig, testForgejoConfig } from "@/lib/api/domains/forgejo-api";
import type { ForgejoConfig, ForgejoRepository } from "@/lib/types/forgejo";

function ForgejoConnection({ workspaceId }: { workspaceId: string }) {
  const [config, setConfig] = useState<ForgejoConfig | null>(null);
  const [origin, setOrigin] = useState("");
  const [token, setToken] = useState("");
  const [repositories, setRepositories] = useState<ForgejoRepository[]>([]);
  const [message, setMessage] = useState("");

  useEffect(() => { void getForgejoConfig({ workspaceId }).then((value) => { setConfig(value); setOrigin(value?.origin ?? ""); }); }, [workspaceId]);
  const options = { workspaceId };
  const test = async () => { const result = await testForgejoConfig({ origin, token }, options); setMessage(result.ok ? `Connected as ${result.username}` : result.error ?? "Connection failed"); };
  const save = async () => { const next = await setForgejoConfig({ origin, token: token || undefined }, options); setConfig(next); setToken(""); setMessage(`Saved connection for ${next.username}`); };
  const loadRepositories = async () => { const result = await listForgejoRepositories(options); setRepositories(result.repositories); };

  return <SettingsCard>
    <CardHeader><CardTitle>Forgejo</CardTitle><CardDescription>Connect this workspace to a Forgejo server. The token is stored in Kandev’s secret store and is never made available to agents.</CardDescription></CardHeader>
    <CardContent className="space-y-3">
      <Input data-testid="forgejo-origin-input" type="url" placeholder="https://forgejo.example" value={origin} onChange={(event) => setOrigin(event.target.value)} />
      <Input data-testid="forgejo-token-input" type="password" placeholder={config?.has_secret ? "Token saved — enter a new value to replace it" : "Forgejo personal access token"} value={token} onChange={(event) => setToken(event.target.value)} />
      <div className="flex flex-wrap gap-2"><Button className="cursor-pointer" type="button" onClick={() => void test()}>Test connection</Button><Button className="cursor-pointer" type="button" onClick={() => void save()}>Save connection</Button>{config?.has_secret ? <Button className="cursor-pointer" variant="outline" type="button" onClick={() => void loadRepositories()}>Load repositories</Button> : null}</div>
      {message ? <p role="status" className="text-sm text-muted-foreground">{message}</p> : null}
      {repositories.length ? <ul className="text-sm space-y-1">{repositories.map((repository) => <li key={repository.full_name}>{repository.full_name}</li>)}</ul> : null}
    </CardContent>
  </SettingsCard>;
}

export function ForgejoIntegrationPage({ workspaceId }: { workspaceId?: string } = {}) {
  return <WorkspaceScopedSection workspaceId={workspaceId}>{(ws) => <ForgejoConnection workspaceId={ws} />}</WorkspaceScopedSection>;
}
