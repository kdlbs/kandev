"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  IconBrandGithub,
  IconCheck,
  IconExternalLink,
  IconEye,
  IconEyeOff,
  IconRefresh,
  IconTrash,
  IconX,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Spinner } from "@kandev/ui/spinner";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { useToast } from "@/components/toast-provider";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import {
  disconnectGitHubAppInstallation,
  disconnectGitHubPersonal,
  disconnectGitHubWorkspace,
  fetchGitHubCLIAccounts,
  setGitHubWorkspaceConnection,
  startGitHubAppInstall,
  startGitHubPersonalConnect,
} from "@/lib/api/domains/github-api";
import { getGitHubPersonalIdentityState } from "@/lib/github-auth";
import type {
  GitHubCLIAccount,
  GitHubConnectionSource,
  GitHubConnectionState,
  GitHubStatus,
} from "@/lib/types/github";

const sourceLabels: Record<GitHubConnectionSource, string> = {
  pat: "Personal access token",
  gh_cli: "GitHub CLI",
  github_app_installation: "GitHub App",
  legacy_shared: "Legacy shared connection",
};

function connectionLabel(status: GitHubStatus): string {
  const connection = status.automation;
  if (!connection) return "";
  return (
    connection.actor?.login ??
    connection.login ??
    connection.installation_account_login ??
    "GitHub App"
  );
}

function automationActor(status: GitHubStatus): string | null {
  if (!status.authenticated) return null;
  return status.automation?.actor?.login ?? null;
}

function StatusLine({ status }: { status: GitHubStatus }) {
  const connection = status.automation;
  if (!connection) {
    return (
      <div className="flex items-center gap-2 text-sm">
        <IconX className="h-4 w-4 shrink-0 text-destructive" />
        <span>No automation connection</span>
      </div>
    );
  }
  const actor = automationActor(status);
  const active = connection.status === "active" && actor !== null;
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2 text-sm">
      {active ? (
        <IconCheck className="h-4 w-4 shrink-0 text-green-500" />
      ) : (
        <IconX className="h-4 w-4 shrink-0 text-destructive" />
      )}
      <span className="min-w-0 break-words font-medium">
        {actor ??
          (connection.status === "active" ? "Authentication unavailable" : connectionLabel(status))}
      </span>
      <Badge variant={active ? "secondary" : "destructive"}>
        {sourceLabels[connection.source]}
      </Badge>
      {connection.status !== "active" && <Badge variant="outline">{connection.status}</Badge>}
    </div>
  );
}

function formatCapability(value: string) {
  return value.replaceAll("_", " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function CapabilityStatus({ status }: { status: GitHubStatus }) {
  const capabilities = Object.entries(status.automation?.capabilities ?? {});
  const missing = [
    ...(status.automation?.missing_capabilities ?? []),
    ...(status.automation?.missing_permissions ?? []),
  ];
  if (capabilities.length === 0 && missing.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-2 pt-1" data-testid="github-capabilities">
      {capabilities.map(([capability, available]) => (
        <Badge key={capability} variant={available ? "outline" : "destructive"}>
          {formatCapability(capability)}
        </Badge>
      ))}
      {missing.map((permission) => (
        <Badge key={permission} variant="destructive">
          Missing {formatCapability(permission)}
        </Badge>
      ))}
    </div>
  );
}

function PATForm({ workspaceId, onSaved }: { workspaceId: string; onSaved: () => void }) {
  const [token, setToken] = useState("");
  const [visible, setVisible] = useState(false);
  const [saving, setSaving] = useState(false);
  const { toast } = useToast();
  const submit = useCallback(
    async (event: React.FormEvent) => {
      event.preventDefault();
      if (!token.trim()) return;
      setSaving(true);
      try {
        await setGitHubWorkspaceConnection(workspaceId, { source: "pat", token: token.trim() });
        setToken("");
        toast({ description: "Workspace GitHub token connected", variant: "success" });
        onSaved();
      } catch (error) {
        toast({
          description: error instanceof Error ? error.message : "Connection failed",
          variant: "error",
        });
      } finally {
        setSaving(false);
      }
    },
    [onSaved, toast, token, workspaceId],
  );

  return (
    <form onSubmit={submit} className="space-y-3 pt-3">
      <Label htmlFor="github-workspace-token">Personal access token</Label>
      <div className="flex flex-col gap-2 sm:flex-row">
        <div className="relative min-w-0 flex-1">
          <Input
            id="github-workspace-token"
            type={visible ? "text" : "password"}
            value={token}
            onChange={(event) => setToken(event.target.value)}
            placeholder="ghp_xxxxxxxxxxxx"
            autoComplete="off"
            className="h-11 pr-11 font-mono"
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="absolute right-0 top-0 h-11 w-11 cursor-pointer"
            onClick={() => setVisible((value) => !value)}
            aria-label={visible ? "Hide token" : "Show token"}
          >
            {visible ? <IconEyeOff className="h-4 w-4" /> : <IconEye className="h-4 w-4" />}
          </Button>
        </div>
        <Button type="submit" disabled={!token.trim() || saving} className="h-11 cursor-pointer">
          {saving && <Spinner className="mr-2 h-4 w-4" />}
          Connect token
        </Button>
      </div>
    </form>
  );
}

function CLIForm({ workspaceId, onSaved }: { workspaceId: string; onSaved: () => void }) {
  const [accounts, setAccounts] = useState<GitHubCLIAccount[]>([]);
  const [selected, setSelected] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const { toast } = useToast();

  useEffect(() => {
    let current = true;
    setLoading(true);
    fetchGitHubCLIAccounts(workspaceId, { cache: "no-store" })
      .then((items) => {
        if (!current) return;
        setAccounts(items);
        const preferred =
          items.find((account) => account.selected) ??
          items.find((account) => account.active) ??
          items[0];
        setSelected(preferred ? `${preferred.host}\n${preferred.login}` : "");
      })
      .catch(() => current && setAccounts([]))
      .finally(() => current && setLoading(false));
    return () => {
      current = false;
    };
  }, [workspaceId]);

  const account = useMemo(() => {
    const [host, login] = selected.split("\n");
    return accounts.find((item) => item.host === host && item.login === login);
  }, [accounts, selected]);

  const connect = useCallback(async () => {
    if (!account) return;
    setSaving(true);
    try {
      await setGitHubWorkspaceConnection(workspaceId, {
        source: "gh_cli",
        host: account.host,
        login: account.login,
      });
      toast({ description: `Connected ${account.login} for this workspace`, variant: "success" });
      onSaved();
    } catch (error) {
      toast({
        description: error instanceof Error ? error.message : "Connection failed",
        variant: "error",
      });
    } finally {
      setSaving(false);
    }
  }, [account, onSaved, toast, workspaceId]);

  return (
    <div className="space-y-3 pt-3">
      <Label htmlFor="github-cli-account">GitHub CLI account</Label>
      <div className="flex flex-col gap-2 sm:flex-row">
        <Select
          value={selected}
          onValueChange={setSelected}
          disabled={loading || accounts.length === 0}
        >
          <SelectTrigger id="github-cli-account" className="h-11 min-w-0 flex-1">
            <SelectValue placeholder={loading ? "Loading accounts..." : "No gh accounts found"} />
          </SelectTrigger>
          <SelectContent>
            {accounts.map((item) => (
              <SelectItem key={`${item.host}:${item.login}`} value={`${item.host}\n${item.login}`}>
                {item.login} ({item.host}){item.active ? " - active" : ""}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button onClick={connect} disabled={!account || saving} className="h-11 cursor-pointer">
          {saving && <Spinner className="mr-2 h-4 w-4" />}
          Use account
        </Button>
      </div>
      {accounts.length === 0 && !loading && (
        <p className="text-xs text-muted-foreground">
          Sign in with <code>gh auth login</code>, then refresh this page.
        </p>
      )}
    </div>
  );
}

async function redirectFrom(start: () => Promise<{ url?: string; URL?: string }>) {
  const response = await start();
  const url = response.url ?? response.URL;
  if (!url) throw new Error("GitHub did not return a redirect URL");
  window.location.assign(url);
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

function automationTab(status: GitHubStatus) {
  if (status.automation?.source === "github_app_installation") return "app";
  if (status.automation?.source === "gh_cli") return "cli";
  return "pat";
}

function AutomationMethodTabs({
  status,
  workspaceId,
  busy,
  onSaved,
  onAppInstall,
}: {
  status: GitHubStatus;
  workspaceId: string;
  busy: boolean;
  onSaved: () => void;
  onAppInstall: () => void;
}) {
  return (
    <Tabs defaultValue={automationTab(status)}>
      <TabsList className="grid h-auto w-full grid-cols-3">
        <TabsTrigger value="pat" className="min-h-11 cursor-pointer">
          PAT
        </TabsTrigger>
        <TabsTrigger value="cli" className="min-h-11 cursor-pointer">
          gh CLI
        </TabsTrigger>
        <TabsTrigger
          value="app"
          className="min-h-11 cursor-pointer"
          disabled={!status.app_available}
        >
          GitHub App
        </TabsTrigger>
      </TabsList>
      <TabsContent value="pat">
        <PATForm workspaceId={workspaceId} onSaved={onSaved} />
      </TabsContent>
      <TabsContent value="cli">
        <CLIForm workspaceId={workspaceId} onSaved={onSaved} />
      </TabsContent>
      <TabsContent value="app" className="space-y-3 pt-3">
        <p className="text-sm text-muted-foreground">
          Install the deployment GitHub App in the organization that owns this workspace's
          repositories.
        </p>
        <Button
          disabled={!status.app_available || busy}
          onClick={onAppInstall}
          className="h-11 w-full cursor-pointer sm:w-auto"
        >
          <IconBrandGithub className="mr-2 h-4 w-4" />
          Install GitHub App
          <IconExternalLink className="ml-2 h-4 w-4" />
        </Button>
      </TabsContent>
    </Tabs>
  );
}

export function GitHubAutomationSettings({ workspaceId }: { workspaceId: string }) {
  const { status, loaded, loading, refresh } = useGitHubStatus(workspaceId);
  const [busy, setBusy] = useState(false);
  const { toast } = useToast();
  const disconnect = useCallback(async () => {
    setBusy(true);
    try {
      if (status?.automation?.source === "github_app_installation") {
        await disconnectGitHubAppInstallation(workspaceId);
      } else {
        await disconnectGitHubWorkspace(workspaceId);
      }
      toast({ description: "Workspace GitHub connection removed", variant: "success" });
      refresh();
    } catch (error) {
      toast({
        description: error instanceof Error ? error.message : "Disconnect failed",
        variant: "error",
      });
    } finally {
      setBusy(false);
    }
  }, [refresh, status?.automation?.source, toast, workspaceId]);
  const installApp = useCallback(() => {
    void redirectFrom(() => startGitHubAppInstall(workspaceId)).catch((error: unknown) =>
      toast({
        description: errorMessage(error, "App installation failed"),
        variant: "error",
      }),
    );
  }, [toast, workspaceId]);

  if (!loaded || loading || !status) return <LoadingStatus />;
  return (
    <div className="space-y-4" data-testid="github-workspace-automation">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0 space-y-1">
          <StatusLine status={status} />
          {status.authenticated && status.automation?.actor && (
            <p className="text-xs text-muted-foreground">
              Agents and background jobs act as {status.automation.actor.login}.
            </p>
          )}
          {status.automation?.last_error && (
            <p className="text-xs text-destructive">{status.automation.last_error}</p>
          )}
          <CapabilityStatus status={status} />
        </div>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="icon"
            onClick={refresh}
            className="h-11 w-11 cursor-pointer"
            aria-label="Refresh GitHub connection"
          >
            <IconRefresh className="h-4 w-4" />
          </Button>
          {status.automation && (
            <Button
              variant="outline"
              onClick={disconnect}
              disabled={busy}
              className="h-11 cursor-pointer text-destructive"
            >
              <IconTrash className="mr-2 h-4 w-4" />
              Disconnect
            </Button>
          )}
        </div>
      </div>

      <AutomationMethodTabs
        status={status}
        workspaceId={workspaceId}
        busy={busy}
        onSaved={refresh}
        onAppInstall={installApp}
      />
    </div>
  );
}

type PersonalIdentityView = {
  active: boolean;
  actor: string;
  description: string;
  personalActive: boolean;
  status: GitHubConnectionState | null;
};

function personalIdentityView(status: GitHubStatus): PersonalIdentityView {
  const appAutomation = status.automation?.source === "github_app_installation";
  const identity = getGitHubPersonalIdentityState(status);
  return {
    active: identity.active,
    actor: identity.actor,
    personalActive: identity.personalOAuthActive,
    status: status.personal?.status ?? null,
    description: appAutomation
      ? "Required for My GitHub, assigned pull requests and issues, and human-attributed actions. App fallback actions are attributed to the App."
      : "My GitHub and user actions use the workspace's human automation identity. A separate personal connection is optional.",
  };
}

function PersonalIdentityStatus({ view }: { view: PersonalIdentityView }) {
  return (
    <>
      <div className="flex min-w-0 flex-wrap items-center gap-2 text-sm">
        {view.active ? (
          <IconCheck className="h-4 w-4 text-green-500" />
        ) : (
          <IconX className="h-4 w-4 text-destructive" />
        )}
        <span className="break-words font-medium">{view.actor}</span>
        {view.personalActive && <Badge variant="secondary">Personal OAuth</Badge>}
        {view.status && view.status !== "active" && (
          <Badge variant="destructive">{view.status}</Badge>
        )}
      </div>
      <p className="text-sm text-muted-foreground">{view.description}</p>
    </>
  );
}

function PersonalIdentityActions({
  status,
  busy,
  onConnect,
  onDisconnect,
}: {
  status: GitHubStatus;
  busy: boolean;
  onConnect: () => void;
  onDisconnect: () => void;
}) {
  return (
    <div className="flex flex-col gap-2 sm:flex-row">
      {status.app_available && status.automation?.source === "github_app_installation" && (
        <Button disabled={busy} onClick={onConnect} className="h-11 cursor-pointer">
          <IconBrandGithub className="mr-2 h-4 w-4" />
          {status.personal ? "Reconnect identity" : "Connect identity"}
          <IconExternalLink className="ml-2 h-4 w-4" />
        </Button>
      )}
      {status.personal && (
        <Button
          variant="outline"
          onClick={onDisconnect}
          disabled={busy}
          className="h-11 cursor-pointer text-destructive"
        >
          <IconTrash className="mr-2 h-4 w-4" />
          Disconnect
        </Button>
      )}
    </div>
  );
}

export function GitHubPersonalSettings({ workspaceId }: { workspaceId: string }) {
  const { status, loaded, loading, refresh } = useGitHubStatus(workspaceId);
  const [busy, setBusy] = useState(false);
  const { toast } = useToast();
  if (!loaded || loading || !status) return <LoadingStatus />;
  const view = personalIdentityView(status);
  const disconnect = async () => {
    setBusy(true);
    try {
      await disconnectGitHubPersonal(workspaceId);
      toast({ description: "Personal GitHub identity disconnected", variant: "success" });
      refresh();
    } catch (error) {
      toast({
        description: error instanceof Error ? error.message : "Disconnect failed",
        variant: "error",
      });
    } finally {
      setBusy(false);
    }
  };
  const connect = () => {
    void redirectFrom(() => startGitHubPersonalConnect(workspaceId)).catch((error: unknown) =>
      toast({
        description: errorMessage(error, "Identity connection failed"),
        variant: "error",
      }),
    );
  };
  return (
    <div className="space-y-4" data-testid="github-personal-identity">
      <PersonalIdentityStatus view={view} />
      {status.personal?.last_error && (
        <p className="text-xs text-destructive">{status.personal.last_error}</p>
      )}
      <PersonalIdentityActions
        status={status}
        busy={busy}
        onConnect={connect}
        onDisconnect={disconnect}
      />
    </div>
  );
}

function LoadingStatus() {
  return (
    <div className="flex min-h-11 items-center gap-2 text-sm text-muted-foreground">
      <Spinner className="h-4 w-4" />
      Checking GitHub connection...
    </div>
  );
}

/** Compatibility export used by older settings entrypoints. */
export function GitHubStatusCard({ workspaceId }: { workspaceId: string }) {
  return <GitHubAutomationSettings workspaceId={workspaceId} />;
}
