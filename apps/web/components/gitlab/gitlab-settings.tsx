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
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { SettingsCard } from "@/components/settings/settings-card";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { clearGitLabToken, fetchGitLabStatus, setGitLabConfig } from "@/lib/api/domains/gitlab-api";
import type { GitLabConfig, GitLabStatus } from "@/lib/types/gitlab";
import { GitLabWatchSettings } from "./watch-settings";
import { GitLabActionPresetsSection } from "./action-presets-section";

const DEFAULT_HOST = "https://gitlab.com";

function StatusBadge({ status }: { status: GitLabStatus | null }) {
  if (!status) return null;
  if (status.authenticated) {
    return (
      <Badge variant="secondary" className="gap-1">
        <IconCheck className="h-3 w-3" /> Connected
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
        <IconAlertTriangle className="h-3 w-3" /> Unreachable
      </Badge>
    );
  }
  if (status.token_configured || status.auth_method !== "none") {
    return (
      <Badge
        variant="outline"
        className="gap-1 border-amber-500/60 text-amber-700 dark:text-amber-300"
      >
        <IconAlertTriangle className="h-3 w-3" /> Reconnect required
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="gap-1">
      <IconX className="h-3 w-3" /> Not connected
    </Badge>
  );
}

// ConnectionErrorAlert renders the per-host transport failure separately from
// the "bad token" path so users see "GitLab is currently unreachable" instead
// of "your token is broken" during an outage. Hidden when the probe succeeded
// or when no token is configured (nothing to probe).
function ConnectionErrorAlert({ status }: { status: GitLabStatus | null }) {
  if (!status?.connection_error) return null;
  return (
    <Alert variant="destructive">
      <IconAlertTriangle className="h-4 w-4" />
      <AlertDescription className="text-sm">
        Couldn&apos;t reach <code className="font-mono text-xs">{status.host}</code>:{" "}
        {status.connection_error}
        <span className="block text-xs opacity-80 mt-1">
          Your token may still be valid — this looks like a network or upstream issue.
        </span>
      </AlertDescription>
    </Alert>
  );
}

function AuthMethodBadge({ method }: { method: GitLabStatus["auth_method"] }) {
  const labels: Record<GitLabStatus["auth_method"], string> = {
    glab_cli: "glab CLI",
    pat: "Personal access token",
    environment: "Environment token",
    none: "Not configured",
    mock: "Mock (test)",
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

function credentialInvalidReason(validHost: boolean, patNeedsToken: boolean) {
  if (!validHost) return "Enter a valid HTTP or HTTPS GitLab host URL.";
  if (patNeedsToken) return "Enter a personal access token to switch to PAT.";
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
      toast({ description: "GitLab authentication method updated", variant: "success" });
      onSaved();
    } catch (error) {
      toast({
        description:
          error instanceof Error ? error.message : "Failed to update authentication method",
        variant: "error",
      });
      throw error;
    }
  }, [host, method, onSaved, toast, token, workspaceId]);
  const patNeedsToken = method === "pat" && !hasToken && !token.trim();
  const validHost = isValidGitLabHost(host);
  useSettingsSaveContributor({
    id: "gitlab-credentials",
    revision: `${method}\0${token}`,
    isDirty,
    canSave: validHost && !patNeedsToken,
    invalidReason: credentialInvalidReason(validHost, patNeedsToken),
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

export function GitLabCredentialsForm(props: GitLabCredentialsFormProps) {
  const draft = useGitLabCredentialDraft(props);
  const [showToken, setShowToken] = useState(false);
  return (
    <div className="space-y-2">
      <p className="text-xs text-muted-foreground">
        Choose a workspace PAT, the local glab CLI login, or an environment-provided token. CLI and
        environment credentials must already be available to the kandev backend.
      </p>
      <Select value={draft.method} onValueChange={draft.selectMethod}>
        <SelectTrigger
          aria-label="Authentication method"
          className="w-full cursor-pointer sm:w-64"
          data-settings-dirty={draft.isDirty}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="pat" className="cursor-pointer">
            Personal access token
          </SelectItem>
          <SelectItem value="glab_cli" className="cursor-pointer">
            glab CLI
          </SelectItem>
          <SelectItem value="environment" className="cursor-pointer">
            Environment token
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
              aria-label={showToken ? "Hide token" : "Show token"}
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
          toast({ description: "GitLab token cleared" });
          onCleared();
        } catch (err) {
          toast({
            description: err instanceof Error ? err.message : "Failed to clear token",
            variant: "error",
          });
        } finally {
          setBusy(false);
        }
      }}
      className="gap-1 cursor-pointer"
    >
      {busy ? <Spinner className="h-3 w-3" /> : <IconTrash className="h-3 w-3" />}
      Clear token
    </Button>
  );
}

type GitLabIntegrationPageProps = {
  workspaceId?: string;
};

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
          Logged in as <span className="font-medium">{status.username}</span>
        </span>
      ) : null}
    </div>
  );
}

function GitLabConnectionCard(props: ConnectionCardProps) {
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
      title="GitLab"
      description="Connect a GitLab account so kandev can open merge requests, read review discussions, and reply to / resolve them on your behalf."
      icon={<IconBrandGitlab className="h-4 w-4" />}
      action={
        <Button
          variant="outline"
          size="sm"
          onClick={() => void reload()}
          disabled={loading}
          className="gap-1 cursor-pointer"
        >
          <IconRefresh className="h-3 w-3" /> Refresh
        </Button>
      }
    >
      <SettingsCard isDirty={authMethodDirty}>
        <CardContent className="space-y-4 py-4">
          <ConnectionErrorAlert status={status} />
          <ConnectionStatusRow status={status} />
          <Separator />
          <div className="space-y-2">
            <p className="text-xs text-muted-foreground">
              GitLab host URL. Override for self-managed instances; leave at the default for
              gitlab.com.
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
