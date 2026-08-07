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
import { useGitHubAppRegistrations } from "@/hooks/domains/github/use-github-app-registrations";
import {
  useTaskGitCredentials,
  type TaskGitCredentialsState,
} from "@/hooks/domains/github/use-task-git-credentials";
import {
  disconnectGitHubPersonal,
  disconnectGitHubWorkspace,
  startGitHubPersonalConnect,
} from "@/lib/api/domains/github-api";
import { getGitHubPersonalIdentityState } from "@/lib/github-auth";
import type {
  GitHubConnectionSource,
  GitHubConnectionState,
  GitHubStatus,
  GitHubAppRegistrationCatalogItem,
} from "@/lib/types/github";
import { GitHubConnectionDialog } from "./github-connection-dialog";
import { GitHubAccessHelp } from "./github-access-help";
import { GitHubPermissionsDialog } from "./github-permissions-dialog";
import { GitHubRateLimitDisplay } from "./github-rate-limit";
import { GitHubTaskAccessSummary } from "./github-task-credentials-section";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

// Keyed by the wire enum, which is never translated; only the label is copy.
// Catalog keys rather than `t()` calls because this is module scope — a `t()`
// here would freeze at the boot locale (see docs/i18n.md).
const sourceLabelKeys: Record<GitHubConnectionSource, string> = {
  pat: "github:personalAccessToken",
  gh_cli: "github:githubCli",
  github_app_installation: "github:githubApp",
  legacy_shared: "github:legacySharedConnection",
};

// `t` is threaded in rather than imported at module level so the label follows a
// locale switch and stays a pure function of its arguments.
function connectionLabel(t: TFunction, status: GitHubStatus): string {
  const connection = status.automation;
  if (!connection) return "";
  // The first three are GitHub account logins — data, never translated.
  return (
    connection.actor?.login ??
    connection.login ??
    connection.installation_account_login ??
    t("github:githubApp")
  );
}

function automationActor(status: GitHubStatus): string | null {
  if (!status.authenticated) return null;
  return status.automation?.actor?.login ?? null;
}

function StatusLine({ status }: { status: GitHubStatus }) {
  const { t } = useTranslation();
  const connection = status.automation;
  if (!connection) {
    return (
      <div className="flex items-center gap-2 text-sm">
        <IconX className="h-4 w-4 shrink-0 text-destructive" />
        <span>{t("github:noAutomationConnection")}</span>
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
          (connection.status === "active"
            ? t("github:authenticationUnavailable")
            : connectionLabel(t, status))}
      </span>
      <Badge variant={active ? "secondary" : "destructive"}>
        {t(sourceLabelKeys[connection.source])}
      </Badge>
      {connection.status !== "active" && <Badge variant="outline">{connection.status}</Badge>}
      <GitHubRateLimitDisplay info={status.rate_limit} />
    </div>
  );
}

async function redirectFrom(t: TFunction, start: () => Promise<{ url?: string; URL?: string }>) {
  const response = await start();
  const url = response.url ?? response.URL;
  // Surfaced to the user through the caller's toast, so it is copy.
  if (!url) throw new Error(t("github:githubDidNotReturnARedirectUrl"));
  window.location.assign(url);
}

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

function AutomationStatusSummary({
  status,
  app,
  taskAccess,
}: {
  status: GitHubStatus;
  app?: GitHubAppRegistrationCatalogItem;
  taskAccess: Omit<TaskGitCredentialsState, "save">;
}) {
  const appAutomation = status.automation?.source === "github_app_installation";
  return (
    <div className="min-w-0 space-y-1">
      <StatusLine status={status} />
      <AutomationActorExplanation status={status} appAutomation={appAutomation} />
      {appAutomation && <AppRegistrationDetails app={app} />}
      <AutomationError status={status} />
      <GitHubTaskAccessSummary {...taskAccess} />
    </div>
  );
}

function AutomationActorExplanation({
  status,
  appAutomation,
}: {
  status: GitHubStatus;
  appAutomation: boolean;
}) {
  const { t } = useTranslation();
  const actor = status.automation?.actor?.login;
  if (!status.authenticated || !actor) return null;
  const humanIdentity = status.effective_personal_actor?.kind === "human";
  return (
    <div className="flex items-start gap-1 text-xs text-muted-foreground">
      <GitHubAccessHelp
        label={t("github:explainWorkspaceGithubIdentity")}
        title={t("github:workspaceGithubIdentity")}
        description={t("github:kandevUsesThisWorkspaceConnectionFor")}
      />
      <div className="min-w-0 space-y-1 pt-3 sm:pt-1">
        <p>
          {appAutomation
            ? t("github:kandevManagedOperationsUseTheGithub", { actor })
            : t("github:kandevManagedOperationsActAs", { actor })}
        </p>
        {!appAutomation && humanIdentity && <p>{t("github:thisAccountAlsoPowersMyGithub")}</p>}
      </div>
    </div>
  );
}

function AppRegistrationDetails({ app }: { app?: GitHubAppRegistrationCatalogItem }) {
  const { t } = useTranslation();
  if (!app) return null;
  return (
    <>
      <div className="flex min-w-0 flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span className="break-words">{app.display_name}</span>
        <Badge variant="outline" className="capitalize">
          {app.visibility}
        </Badge>
        <Badge variant="outline" className="capitalize">
          {t("github:webhook")} {app.webhook_status}
        </Badge>
        <span>{app.source === "managed" ? t("github:createdByKandev") : t("github:imported")}</span>
      </div>
      {app.shared && (
        <p className="text-xs text-amber-600 dark:text-amber-400">
          {t("github:thisAppRegistrationIsShared", { count: app.workspace_binding_count })}
        </p>
      )}
    </>
  );
}

function AutomationError({ status }: { status: GitHubStatus }) {
  if (!status.automation?.last_error) return null;
  return <p className="text-xs text-destructive">{status.automation.last_error}</p>;
}

function AutomationActions({
  status,
  workspaceId,
  busy,
  refreshing,
  onDisconnect,
  onRefresh,
  taskAccess,
}: {
  status: GitHubStatus;
  workspaceId: string;
  busy: boolean;
  refreshing: boolean;
  onDisconnect: () => void;
  onRefresh: () => void;
  taskAccess: TaskGitCredentialsState;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap gap-2">
      <GitHubPermissionsDialog status={status} />
      <GitHubConnectionDialog
        status={status}
        workspaceId={workspaceId}
        onSaved={onRefresh}
        taskAccess={taskAccess}
      />
      <Button
        variant="outline"
        size="icon"
        onClick={onRefresh}
        disabled={refreshing}
        aria-busy={refreshing}
        className="h-11 w-11 cursor-pointer"
        aria-label={t("github:refreshGithubConnection")}
      >
        <IconRefresh className={`h-4 w-4 ${refreshing ? "animate-spin" : ""}`} />
      </Button>
      {status.automation && (
        <Button
          variant="outline"
          onClick={onDisconnect}
          disabled={busy}
          className="h-11 cursor-pointer text-destructive"
        >
          <IconTrash className="mr-2 h-4 w-4" />
          {t("github:disconnect")}
        </Button>
      )}
    </div>
  );
}

export function GitHubAutomationSettings({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const { status, loaded, loading, refresh } = useGitHubStatus(workspaceId);
  const appRegistrations = useGitHubAppRegistrations(workspaceId);
  const taskAccess = useTaskGitCredentials(workspaceId);
  const [busy, setBusy] = useState(false);
  const { toast } = useToast();
  const disconnect = useCallback(async () => {
    setBusy(true);
    try {
      await disconnectGitHubWorkspace(workspaceId);
      toast({ description: t("github:workspaceGithubConnectionRemoved"), variant: "success" });
      refresh();
    } catch (error) {
      toast({
        description: error instanceof Error ? error.message : t("github:disconnectFailed"),
        variant: "error",
      });
    } finally {
      setBusy(false);
    }
  }, [refresh, toast, workspaceId]);
  if (!loaded || !status) return <LoadingStatus />;
  const activeRegistrationId =
    status.app_registration?.id ?? status.automation?.app_registration_id;
  const activeApp = appRegistrations.registrations.find((item) => item.id === activeRegistrationId);
  return (
    <div
      className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"
      data-testid="github-workspace-automation"
    >
      <AutomationStatusSummary status={status} app={activeApp} taskAccess={taskAccess} />
      <AutomationActions
        status={status}
        workspaceId={workspaceId}
        busy={busy}
        refreshing={loading}
        onDisconnect={disconnect}
        onRefresh={refresh}
        taskAccess={taskAccess}
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
  const { t } = useTranslation();
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2 text-sm">
      {view.active ? (
        <IconCheck className="h-4 w-4 text-green-500" />
      ) : (
        <IconX className="h-4 w-4 text-destructive" />
      )}
      <span className="break-words font-medium">{view.actor}</span>
      {view.personalActive && <Badge variant="secondary">{t("github:personalOauth")}</Badge>}
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
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2 sm:flex-row">
      {status.app_available && status.automation?.source === "github_app_installation" && (
        <Button disabled={busy} onClick={onConnect} className="h-11 cursor-pointer">
          <IconBrandGithub className="mr-2 h-4 w-4" />
          {status.personal ? t("github:reconnectIdentity") : t("github:connectIdentity")}
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
          {t("github:disconnect")}
        </Button>
      )}
    </div>
  );
}

export function GitHubPersonalSettings({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const { status, loaded, refresh } = useGitHubStatus(workspaceId);
  const [busy, setBusy] = useState(false);
  const { toast } = useToast();
  if (!loaded || !status) return null;
  const view = personalIdentityView(status);
  const appAutomation = status.automation?.source === "github_app_installation";
  if (!status.automation) return null;
  if (!appAutomation) return null;
  const disconnect = async () => {
    setBusy(true);
    try {
      await disconnectGitHubPersonal(workspaceId);
      toast({ description: t("github:personalGithubIdentityDisconnected"), variant: "success" });
      refresh();
    } catch (error) {
      toast({
        description: error instanceof Error ? error.message : t("github:disconnectFailed"),
        variant: "error",
      });
    } finally {
      setBusy(false);
    }
  };
  const connect = () => {
    void redirectFrom(t, () => startGitHubPersonalConnect(workspaceId)).catch((error: unknown) =>
      toast({
        description: errorMessage(error, t("github:identityConnectionFailed")),
        variant: "error",
      }),
    );
  };
  return (
    <SettingsSection
      title={t("github:myGithubIdentity")}
      description={t("github:optionallyConnectYourGithubUserFor")}
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
  const { t } = useTranslation();
  return (
    <div className="flex min-h-11 items-center gap-2 text-sm text-muted-foreground">
      <Spinner className="h-4 w-4" />
      {t("github:checkingGithubConnection")}
    </div>
  );
}

/** Compatibility export used by older settings entrypoints. */
export function GitHubStatusCard({ workspaceId }: { workspaceId: string }) {
  return <GitHubAutomationSettings workspaceId={workspaceId} />;
}
