"use client";

import { useCallback, useEffect, useState } from "react";
import {
  IconAlertTriangle,
  IconBrandGitlab,
  IconCheck,
  IconEye,
  IconEyeOff,
  IconKey,
  IconRefresh,
  IconTrash,
  IconWorld,
  IconX,
} from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Separator } from "@kandev/ui/separator";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Spinner } from "@kandev/ui/spinner";
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
import { GitLabEnabledControl } from "@/components/gitlab/gitlab-enabled-control";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { SettingsCard } from "@/components/settings/settings-card";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { clearGitLabToken, fetchGitLabStatus, setGitLabConfig } from "@/lib/api/domains/gitlab-api";
import type { GitLabConfig, GitLabStatus } from "@/lib/types/gitlab";
import { GitLabWatchSettings } from "./watch-settings";
import { GitLabActionPresetsSection } from "./action-presets-section";
import { Trans, useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { INTEGRATION_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/integrations";

const DEFAULT_HOST = "https://gitlab.com";
// The bare hostname as it reads mid-sentence. Interpolated rather than written
// into the catalog so no locale — including pseudo — can transliterate it into a
// hostname that does not resolve.
const DEFAULT_HOST_NAME = "gitlab.com";

function StatusBadge({ status }: { status: GitLabStatus | null }) {
  const { t } = useTranslation();
  if (!status) return null;
  if (status.authenticated) {
    return (
      <Badge variant="secondary" className="gap-1">
        <IconCheck className="h-3 w-3" /> {t("gitlab:connected")}
      </Badge>
    );
  }
  // A non-empty connection_error means the probe failed for transport reasons
  // (network / 5xx / parse) — distinct from "no token configured", which has
  // an empty connection_error and authenticated=false.
  if (status.connection_error) {
    return (
      <Badge
        variant="outline"
        className="gap-1 border-amber-500/60 text-amber-700 dark:text-amber-300"
      >
        <IconAlertTriangle className="h-3 w-3" /> {t("gitlab:unreachable")}
      </Badge>
    );
  }
  if (status.token_configured || status.auth_method !== "none") {
    return (
      <Badge
        variant="outline"
        className="gap-1 border-amber-500/60 text-amber-700 dark:text-amber-300"
      >
        <IconAlertTriangle className="h-3 w-3" /> {t("gitlab:reconnectRequired")}
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="gap-1">
      <IconX className="h-3 w-3" /> {t("gitlab:notConnected")}
    </Badge>
  );
}

// ConnectionErrorAlert renders the per-host transport failure separately from
// the "bad token" path so users see "GitLab is currently unreachable" instead
// of "your token is broken" during an outage. Hidden when the probe succeeded
// or when no token is configured (nothing to probe).
function ConnectionErrorAlert({ status }: { status: GitLabStatus | null }) {
  const { t } = useTranslation();
  if (!status?.connection_error) return null;
  return (
    <Alert variant="destructive">
      <IconAlertTriangle className="h-4 w-4" />
      <AlertDescription className="text-sm">
        {/* One message rather than a stem plus the host: where the host and the
            upstream error sit in the sentence is the translator's call. Both are
            server data, so they travel through `values`. */}
        <Trans
          i18nKey="gitlab:couldNotReachHost"
          values={{ host: status.host, error: status.connection_error }}
        >
          Couldn&apos;t reach <code className="font-mono text-xs">{status.host}</code>:{" "}
          {status.connection_error}
        </Trans>
        <span className="block text-xs opacity-80 mt-1">
          {t("gitlab:yourTokenMayStillBeValid")}
        </span>
      </AlertDescription>
    </Alert>
  );
}

// The record keys are the wire `auth_method` values the backend sends and must
// never be translated; only the values are copy. Built inside the component so
// `t()` runs at render — a module-scope table would freeze at the boot locale.
// The `?? method` fallback deliberately echoes the raw wire value for a method
// this build does not know about.
function AuthMethodBadge({ method }: { method: GitLabStatus["auth_method"] }) {
  const { t } = useTranslation();
  const labels: Record<GitLabStatus["auth_method"], string> = {
    glab_cli: t("gitlab:glabCli"),
    pat: t("gitlab:personalAccessToken"),
    environment: t("gitlab:environmentToken"),
    none: t("gitlab:notConfigured"),
    mock: t("gitlab:mockTest"),
  };
  return <Badge variant="outline">{labels[method] ?? method}</Badge>;
}

function HostForm({
  host,
  baseline,
  onHostChange,
}: {
  host: string;
  baseline: string;
  onHostChange: (host: string) => void;
}) {
  const isDirty = host !== baseline;
  return (
    <div className="flex gap-2 items-center">
      <IconWorld className="h-4 w-4 text-muted-foreground shrink-0" />
      <Input
        data-testid="gitlab-host-input"
        type="url"
        placeholder={DEFAULT_HOST}
        value={host}
        data-settings-dirty={isDirty}
        onChange={(event) => onHostChange(event.target.value)}
        className="font-mono text-sm"
      />
    </div>
  );
}

type GitLabCredentialsFormProps = {
  initial: GitLabConfig["auth_method"];
  initialHost: string;
  host: string;
  workspaceId: string;
  hasToken: boolean;
  onChange: (method: GitLabConfig["auth_method"]) => void;
  onSaved: () => void;
  onDirtyChange: (isDirty: boolean) => void;
  onHostChange: (host: string) => void;
};

function isValidGitLabHost(host: string): boolean {
  try {
    const url = new URL(host.trim());
    return (
      (url.protocol === "http:" || url.protocol === "https:") && !url.username && !url.password
    );
  } catch {
    return false;
  }
}

// `t` is threaded in: this is a plain function, and the guard only inspects JSX,
// so a literal returned from here would never be reported.
function credentialInvalidReason(t: TFunction, validHost: boolean, patNeedsToken: boolean) {
  if (!validHost) return t("gitlab:enterAValidHttpOrHttpsGitlabHost");
  if (patNeedsToken) return t("gitlab:enterAPersonalAccessTokenToSwitch");
  return undefined;
}

function useGitLabCredentialDraft({
  initial,
  initialHost,
  host,
  workspaceId,
  hasToken,
  onChange,
  onSaved,
  onDirtyChange,
  onHostChange,
}: GitLabCredentialsFormProps) {
  const { t } = useTranslation();
  const [method, setMethod] = useState(initial);
  const [baseline, setBaseline] = useState(initial);
  const [syncedInitial, setSyncedInitial] = useState(initial);
  const [hostBaseline, setHostBaseline] = useState(initialHost);
  const [token, setToken] = useState("");
  const { toast } = useToast();
  const isDirty = method !== baseline || Boolean(token) || host.trim() !== hostBaseline;
  useEffect(() => onDirtyChange(isDirty), [isDirty, onDirtyChange]);
  if (initial !== syncedInitial && method === baseline) {
    setSyncedInitial(initial);
    setBaseline(initial);
    setMethod(initial);
  }
  if (initialHost !== hostBaseline && !token && method === baseline) {
    setHostBaseline(initialHost);
  }
  const save = useCallback(async () => {
    try {
      const submittedToken = token.trim();
      await setGitLabConfig(
        {
          host: host.trim(),
          auth_method: method,
          ...(method === "pat" && submittedToken ? { token: submittedToken } : {}),
        },
        { workspaceId },
      );
      setBaseline(method);
      setHostBaseline(host.trim());
      setToken((current) => (current.trim() === submittedToken ? "" : current));
      toast({ description: t("gitlab:gitlabAuthenticationMethodUpdated"), variant: "success" });
      onSaved();
    } catch (error) {
      toast({
        description:
          error instanceof Error ? error.message : t("gitlab:failedToUpdateAuthenticationMethod"),
        variant: "error",
      });
      throw error;
    }
  }, [host, method, onSaved, t, toast, token, workspaceId]);
  const patNeedsToken = method === "pat" && !hasToken && !token.trim();
  const validHost = isValidGitLabHost(host);
  useSettingsSaveContributor({
    id: "gitlab-credentials",
    revision: JSON.stringify({ host: host.trim(), method, token }),
    isDirty,
    canSave: validHost && !patNeedsToken,
    invalidReason: credentialInvalidReason(t, validHost, patNeedsToken),
    save,
    discard: () => {
      setMethod(baseline);
      setToken("");
      onHostChange(hostBaseline);
      onChange(baseline);
    },
  });
  const selectMethod = (value: string) => {
    const next = value as GitLabConfig["auth_method"];
    setMethod(next);
    onChange(next);
  };
  return { method, token, setToken, selectMethod, isDirty };
}

/** Credential form (PAT or OAuth) for connecting a workspace's GitLab account. */
export function GitLabCredentialsForm(props: GitLabCredentialsFormProps) {
  const { t } = useTranslation();
  const draft = useGitLabCredentialDraft(props);
  const [showToken, setShowToken] = useState(false);
  return (
    <div className="space-y-2">
      <p className="text-xs text-muted-foreground">{t("gitlab:chooseAWorkspacePatTheLocal")}</p>
      <Select value={draft.method} onValueChange={draft.selectMethod}>
        <SelectTrigger
          aria-label={t("gitlab:authenticationMethod")}
          className="w-full cursor-pointer sm:w-64"
          data-settings-dirty={draft.isDirty}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="pat" className="cursor-pointer">
            {t("gitlab:personalAccessToken")}
          </SelectItem>
          <SelectItem value="glab_cli" className="cursor-pointer">
            {t("gitlab:glabCli")}
          </SelectItem>
          <SelectItem value="environment" className="cursor-pointer">
            {t("gitlab:environmentToken")}
          </SelectItem>
        </SelectContent>
      </Select>
      {draft.method === "pat" ? (
        <div className="flex items-center gap-2">
          <IconKey className="h-4 w-4 shrink-0 text-muted-foreground" />
          <div className="relative flex-1">
            <Input
              data-testid="gitlab-token-input"
              type={showToken ? "text" : "password"}
              placeholder="glpat-xxxxxxxxxxxxxxxxxxxx"
              value={draft.token}
              data-settings-dirty={Boolean(draft.token)}
              onChange={(event) => draft.setToken(event.target.value)}
              className="font-mono text-sm pr-9"
              autoComplete="off"
            />
            <button
              type="button"
              onClick={() => setShowToken((value) => !value)}
              className="absolute right-2 top-1/2 -translate-y-1/2 cursor-pointer text-muted-foreground hover:text-foreground"
              aria-label={showToken ? t("gitlab:hideToken") : t("gitlab:showToken")}
            >
              {showToken ? <IconEyeOff className="h-4 w-4" /> : <IconEye className="h-4 w-4" />}
            </button>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function ClearTokenButton({
  workspaceId,
  onCleared,
}: {
  workspaceId: string;
  onCleared: () => void;
}) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  const { toast } = useToast();
  return (
    <Button
      variant="outline"
      size="sm"
      disabled={busy}
      onClick={async () => {
        setBusy(true);
        try {
          await clearGitLabToken({ workspaceId });
          toast({ description: t("gitlab:gitlabTokenCleared") });
          onCleared();
        } catch (err) {
          toast({
            description: err instanceof Error ? err.message : t("gitlab:failedToClearToken"),
            variant: "error",
          });
        } finally {
          setBusy(false);
        }
      }}
      className="gap-1 cursor-pointer"
    >
      {busy ? <Spinner className="h-3 w-3" /> : <IconTrash className="h-3 w-3" />}
      {t("gitlab:clearToken")}
    </Button>
  );
}

type GitLabIntegrationPageProps = {
  workspaceId?: string;
};

/** GitLab's own settings page: connection, action presets, and watch settings. */
export function GitLabIntegrationPage({ workspaceId }: GitLabIntegrationPageProps = {}) {
  return (
    <WorkspaceScopedSection workspaceId={workspaceId}>
      {(ws) => (
        <div key={ws} className="space-y-8">
          <GitLabConnectionSection workspaceId={ws} />
          <GitLabActionPresetsSection workspaceId={ws} />
          <GitLabWatchSettings workspaceId={ws} />
        </div>
      )}
    </WorkspaceScopedSection>
  );
}

function editableAuthMethod(status: GitLabStatus | null): GitLabConfig["auth_method"] {
  return status?.auth_method === "glab_cli" || status?.auth_method === "environment"
    ? status.auth_method
    : "pat";
}

type ConnectionCardProps = {
  workspaceId: string;
  status: GitLabStatus | null;
  loading: boolean;
  authMethodDirty: boolean;
  hostDraft: string;
  authMethodDraft: GitLabConfig["auth_method"];
  setHostDraft: (host: string) => void;
  setAuthMethodDraft: (method: GitLabConfig["auth_method"]) => void;
  setAuthMethodDirty: (dirty: boolean) => void;
  reload: () => Promise<void>;
};

function ConnectionStatusRow({ status }: { status: GitLabStatus | null }) {
  return (
    <div className="flex items-center justify-between">
      <div className="flex items-center gap-2">
        <StatusBadge status={status} />
        {status && <AuthMethodBadge method={status.auth_method} />}
        {status?.glab_version ? (
          <Badge variant="outline" className="font-mono text-xs">
            glab {status.glab_version}
          </Badge>
        ) : null}
      </div>
      {status?.username ? (
        <span className="text-xs text-muted-foreground">
          <Trans i18nKey="gitlab:loggedInAs" values={{ username: status.username }}>
            Logged in as <span className="font-medium">{status.username}</span>
          </Trans>
        </span>
      ) : null}
    </div>
  );
}

function GitLabConnectionCard(props: ConnectionCardProps) {
  const { t } = useTranslation();
  const {
    workspaceId,
    status,
    loading,
    authMethodDirty,
    hostDraft,
    authMethodDraft,
    setHostDraft,
    setAuthMethodDraft,
    setAuthMethodDirty,
    reload,
  } = props;
  return (
    <SettingsSection
      discoveryTargetId={INTEGRATION_SETTINGS_TARGETS.gitlab}
      title="GitLab"
      description={t("gitlab:connectAGitlabAccountSoKandev")}
      icon={<IconBrandGitlab className="h-4 w-4" />}
      action={
        <div className="flex items-center gap-2">
          <GitLabEnabledControl />
          <Button
            variant="outline"
            size="sm"
            onClick={() => void reload()}
            disabled={loading}
            className="gap-1 cursor-pointer"
          >
            <IconRefresh className="h-3 w-3" /> {t("gitlab:refresh")}
          </Button>
        </div>
      }
    >
      <SettingsCard isDirty={authMethodDirty}>
        <CardContent className="space-y-4 py-4">
          <ConnectionErrorAlert status={status} />
          <ConnectionStatusRow status={status} />
          <Separator />
          <div className="space-y-2">
            <p className="text-xs text-muted-foreground">
              {t("gitlab:gitlabHostUrlOverrideForSelf", { host: DEFAULT_HOST_NAME })}
            </p>
            <HostForm
              host={hostDraft}
              baseline={status?.host ?? DEFAULT_HOST}
              onHostChange={setHostDraft}
            />
          </div>
          <Separator />
          <GitLabCredentialsForm
            initial={editableAuthMethod(status)}
            initialHost={status?.host ?? DEFAULT_HOST}
            host={hostDraft}
            workspaceId={workspaceId}
            hasToken={Boolean(status?.token_configured)}
            onChange={setAuthMethodDraft}
            onSaved={() => void reload()}
            onDirtyChange={setAuthMethodDirty}
            onHostChange={setHostDraft}
          />
          {authMethodDraft === "pat" && status?.token_configured ? (
            <div className="flex justify-end">
              <ClearTokenButton workspaceId={workspaceId} onCleared={() => void reload()} />
            </div>
          ) : null}
        </CardContent>
      </SettingsCard>
    </SettingsSection>
  );
}

function GitLabConnectionSection({ workspaceId }: { workspaceId: string }) {
  const [status, setStatus] = useState<GitLabStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [authMethodDirty, setAuthMethodDirty] = useState(false);
  const [hostDraft, setHostDraft] = useState(DEFAULT_HOST);
  const [authMethodDraft, setAuthMethodDraft] = useState<GitLabConfig["auth_method"]>("pat");

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const next = await fetchGitLabStatus({ cache: "no-store", workspaceId });
      setStatus(next);
      setHostDraft(next?.host ?? DEFAULT_HOST);
      setAuthMethodDraft(editableAuthMethod(next));
    } catch {
      setStatus(null);
    } finally {
      setLoading(false);
    }
  }, [workspaceId]);

  useEffect(() => {
    void reload();
  }, [reload]);

  return (
    <GitLabConnectionCard
      workspaceId={workspaceId}
      status={status}
      loading={loading}
      authMethodDirty={authMethodDirty}
      hostDraft={hostDraft}
      authMethodDraft={authMethodDraft}
      setHostDraft={setHostDraft}
      setAuthMethodDraft={setAuthMethodDraft}
      setAuthMethodDirty={setAuthMethodDirty}
      reload={reload}
    />
  );
}
