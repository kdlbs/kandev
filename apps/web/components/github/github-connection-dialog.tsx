"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  IconBrandGithub,
  IconExternalLink,
  IconEye,
  IconEyeOff,
  IconPlug,
  IconSettings,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Spinner } from "@kandev/ui/spinner";
import Link from "@/components/routing/app-link";
import { useToast } from "@/components/toast-provider";
import { fetchGitHubCLIAccounts, setGitHubWorkspaceConnection } from "@/lib/api/domains/github-api";
import type { GitHubCLIAccount, GitHubStatus } from "@/lib/types/github";

type AutomationMethod = "pat" | "cli" | "app";

const methodDescriptions: Record<AutomationMethod, string> = {
  pat: "A human GitHub account acts for background jobs and managed agents in this workspace.",
  cli: "One exact human account already authenticated by GitHub CLI acts for this workspace.",
  app: "The deployment-owned App acts as organization-managed automation with short-lived credentials.",
};

function methodForStatus(status: GitHubStatus): AutomationMethod {
  if (status.automation?.source === "github_app_installation") return "app";
  if (status.automation?.source === "gh_cli") return "cli";
  return "pat";
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
    <form onSubmit={submit} className="space-y-3">
      <Label htmlFor="github-workspace-token">Personal access token</Label>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-stretch">
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
    <div className="space-y-3">
      <Label htmlFor="github-cli-account">GitHub CLI account</Label>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-stretch">
        <Select
          value={selected}
          onValueChange={setSelected}
          disabled={loading || accounts.length === 0}
        >
          <SelectTrigger id="github-cli-account" className="min-h-11 min-w-0 flex-1">
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
          Sign in with <code>gh auth login</code>, then reopen this dialog.
        </p>
      )}
    </div>
  );
}

function AppForm({
  available,
  busy,
  onInstall,
}: {
  available: boolean;
  busy: boolean;
  onInstall: () => void;
}) {
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        {available
          ? "Install the deployment GitHub App in the organization that owns this workspace's repositories."
          : "This Kandev deployment does not have a GitHub App yet. Create it once in System Settings, then return here to install it for this workspace."}
      </p>
      {available ? (
        <Button
          disabled={busy}
          onClick={onInstall}
          className="h-11 w-full cursor-pointer sm:w-auto"
          data-testid="github-app-install-button"
        >
          <IconBrandGithub className="mr-2 h-4 w-4" />
          Install GitHub App
          <IconExternalLink className="ml-2 h-4 w-4" />
        </Button>
      ) : (
        <Button
          asChild
          className="h-11 w-full sm:w-auto"
          data-testid="github-app-system-setup-link"
        >
          <Link href="/settings/system/github-app">
            Set up deployment App
            <IconSettings className="ml-2 h-4 w-4" />
          </Link>
        </Button>
      )}
    </div>
  );
}

export function GitHubConnectionDialog({
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
  const [open, setOpen] = useState(false);
  const [method, setMethod] = useState<AutomationMethod>(() => methodForStatus(status));
  const connected = Boolean(status.automation);

  const saved = useCallback(() => {
    onSaved();
    setOpen(false);
  }, [onSaved]);

  const handleOpenChange = useCallback(
    (nextOpen: boolean) => {
      if (nextOpen) setMethod(methodForStatus(status));
      setOpen(nextOpen);
    },
    [status],
  );

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>
        <Button variant={connected ? "outline" : "default"} className="h-11 cursor-pointer">
          <IconPlug className="mr-2 h-4 w-4" />
          {connected ? "Change connection" : "Connect GitHub"}
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{connected ? "Change GitHub connection" : "Connect GitHub"}</DialogTitle>
          <DialogDescription>
            Workspace automation uses this connection for background jobs and managed agent GitHub
            access. PAT and CLI act as people; the deployment App acts as automation. Changing it
            takes effect immediately.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="github-connection-method">Connection method</Label>
            <Select value={method} onValueChange={(value) => setMethod(value as AutomationMethod)}>
              <SelectTrigger id="github-connection-method" className="min-h-11 w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="pat">Personal access token</SelectItem>
                <SelectItem value="cli">GitHub CLI</SelectItem>
                <SelectItem value="app">GitHub App</SelectItem>
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">{methodDescriptions[method]}</p>
          </div>
          {method === "pat" && <PATForm workspaceId={workspaceId} onSaved={saved} />}
          {method === "cli" && <CLIForm workspaceId={workspaceId} onSaved={saved} />}
          {method === "app" && (
            <AppForm
              available={Boolean(status.app_available)}
              busy={busy}
              onInstall={onAppInstall}
            />
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
