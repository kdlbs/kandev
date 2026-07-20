"use client";

import { useCallback, useState } from "react";
import {
  IconBrandGithub,
  IconCheck,
  IconExternalLink,
  IconRefresh,
  IconTrash,
  IconX,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { SettingsSection } from "@/components/settings/settings-section";
import { useToast } from "@/components/toast-provider";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import {
  disconnectGitHubAppInstallation,
  disconnectGitHubPersonal,
  disconnectGitHubWorkspace,
  startGitHubAppInstall,
  startGitHubPersonalConnect,
} from "@/lib/api/domains/github-api";
import { getGitHubPersonalIdentityState } from "@/lib/github-auth";
import type {
  GitHubConnectionSource,
  GitHubConnectionState,
  GitHubStatus,
} from "@/lib/types/github";
import { GitHubConnectionDialog } from "./github-connection-dialog";
import { GitHubPermissionsDialog } from "./github-permissions-dialog";

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

async function redirectFrom(start: () => Promise<{ url?: string; URL?: string }>) {
  const response = await start();
  const url = response.url ?? response.URL;
  if (!url) throw new Error("GitHub did not return a redirect URL");
  window.location.assign(url);
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

function AutomationStatusSummary({ status }: { status: GitHubStatus }) {
  const appAutomation = status.automation?.source === "github_app_installation";
  const actor = status.automation?.actor?.login;
  return (
    <div className="min-w-0 space-y-1">
      <StatusLine status={status} />
      {status.authenticated && actor && (
        <p className="text-xs text-muted-foreground">
          {appAutomation
            ? `Kandev-managed operations use the GitHub App installed for ${actor}.`
            : `Kandev-managed operations act as ${actor}.`}
        </p>
      )}
      {!appAutomation && status.effective_personal_actor?.kind === "human" && (
        <p className="text-xs text-muted-foreground">
          This account also powers My GitHub and user-triggered actions.
        </p>
      )}
      {status.automation?.last_error && (
        <p className="text-xs text-destructive">{status.automation.last_error}</p>
      )}
    </div>
  );
}

function AutomationActions({
  status,
  workspaceId,
  busy,
  onDisconnect,
  onRefresh,
  onAppInstall,
}: {
  status: GitHubStatus;
  workspaceId: string;
  busy: boolean;
  onDisconnect: () => void;
  onRefresh: () => void;
  onAppInstall: () => void;
}) {
  return (
    <div className="flex flex-wrap gap-2">
      <GitHubPermissionsDialog status={status} />
      <GitHubConnectionDialog
        status={status}
        workspaceId={workspaceId}
        busy={busy}
        onSaved={onRefresh}
        onAppInstall={onAppInstall}
      />
      <Button
        variant="outline"
        size="icon"
        onClick={onRefresh}
        className="h-11 w-11 cursor-pointer"
        aria-label="Refresh GitHub connection"
      >
        <IconRefresh className="h-4 w-4" />
      </Button>
      {status.automation && (
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
    <div
      className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"
      data-testid="github-workspace-automation"
    >
      <AutomationStatusSummary status={status} />
      <AutomationActions
        status={status}
        workspaceId={workspaceId}
        busy={busy}
        onDisconnect={disconnect}
        onRefresh={refresh}
        onAppInstall={installApp}
      />
    </div>
  );
}

type PersonalIdentityView = {
  active: boolean;
  actor: string;
  personalActive: boolean;
  status: GitHubConnectionState | null;
};

function personalIdentityView(status: GitHubStatus): PersonalIdentityView {
  const identity = getGitHubPersonalIdentityState(status);
  return {
    active: identity.active,
    actor: identity.actor,
    personalActive: identity.personalOAuthActive,
    status: status.personal?.status ?? null,
  };
}

function PersonalIdentityStatus({ view }: { view: PersonalIdentityView }) {
  return (
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
  if (!loaded || loading || !status) {
    return (
      <SettingsSection
        title="Personal GitHub identity"
        description="Connect your GitHub user for My GitHub and human-attributed actions. Without it, automation continues as the App."
      >
        <LoadingStatus />
      </SettingsSection>
    );
  }
  if (status.automation?.source !== "github_app_installation") return null;
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
    <SettingsSection
      title="Personal GitHub identity"
      description="Connect your GitHub user for My GitHub and human-attributed actions. Without it, automation continues as the App."
    >
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
    </SettingsSection>
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
