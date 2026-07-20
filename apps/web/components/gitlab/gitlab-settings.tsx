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
import { Spinner } from "@kandev/ui/spinner";
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { SettingsCard } from "@/components/settings/settings-card";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import {
  clearGitLabToken,
  configureGitLabHost,
  configureGitLabToken,
  fetchGitLabStatus,
} from "@/lib/api/domains/gitlab-api";
import type { GitLabStatus } from "@/lib/types/gitlab";

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
    none: "Not configured",
    mock: "Mock (test)",
  };
  return <Badge variant="outline">{labels[method] ?? method}</Badge>;
}

function HostForm({
  initial,
  onSaved,
  onDirtyChange,
}: {
  initial: string;
  onSaved: () => void;
  onDirtyChange: (isDirty: boolean) => void;
}) {
  const [host, setHost] = useState(initial);
  const [baseline, setBaseline] = useState(initial);
  const [syncedInitial, setSyncedInitial] = useState(initial);
  const { toast } = useToast();
  const isDirty = host !== baseline;

  useEffect(() => onDirtyChange(isDirty), [isDirty, onDirtyChange]);

  if (initial !== syncedInitial && host === baseline) {
    setSyncedInitial(initial);
    setBaseline(initial);
    setHost(initial);
  }

  const save = useCallback(async () => {
    const submitted = host.trim();
    try {
      await configureGitLabHost(submitted);
      setBaseline(submitted);
      setHost((current) => (current.trim() === submitted ? submitted : current));
      toast({ description: "GitLab host updated", variant: "success" });
      onSaved();
    } catch (err) {
      toast({
        description: err instanceof Error ? err.message : "Failed to update host",
        variant: "error",
      });
      throw err;
    }
  }, [host, toast, onSaved]);
  const discard = useCallback(() => setHost(baseline), [baseline]);
  const validHost = (() => {
    try {
      const url = new URL(host.trim());
      return (
        (url.protocol === "http:" || url.protocol === "https:") && !url.username && !url.password
      );
    } catch {
      return false;
    }
  })();

  useSettingsSaveContributor({
    id: "gitlab-host",
    revision: host,
    isDirty,
    canSave: validHost,
    invalidReason: validHost ? undefined : "Enter a valid HTTP or HTTPS GitLab host URL.",
    save,
    discard,
  });

  return (
    <div className="flex gap-2 items-center">
      <IconWorld className="h-4 w-4 text-muted-foreground shrink-0" />
      <Input
        type="url"
        placeholder={DEFAULT_HOST}
        value={host}
        data-settings-dirty={isDirty}
        onChange={(e) => setHost(e.target.value)}
        className="font-mono text-sm"
      />
    </div>
  );
}

function TokenForm({
  onSuccess,
  onDirtyChange,
}: {
  onSuccess: () => void;
  onDirtyChange: (isDirty: boolean) => void;
}) {
  const [token, setToken] = useState("");
  const [showToken, setShowToken] = useState(false);
  const { toast } = useToast();
  const isDirty = Boolean(token);

  useEffect(() => onDirtyChange(isDirty), [isDirty, onDirtyChange]);

  const save = useCallback(async () => {
    const submitted = token.trim();
    try {
      await configureGitLabToken(submitted);
      toast({ description: "GitLab token configured", variant: "success" });
      setToken((current) => (current.trim() === submitted ? "" : current));
      onSuccess();
    } catch (err) {
      toast({
        description: err instanceof Error ? err.message : "Failed to save token",
        variant: "error",
      });
      throw err;
    }
  }, [token, toast, onSuccess]);
  const discard = useCallback(() => setToken(""), []);

  useSettingsSaveContributor({
    id: "gitlab-token",
    revision: token,
    isDirty,
    canSave: Boolean(token.trim()),
    invalidReason: token && !token.trim() ? "A GitLab token is required." : undefined,
    save,
    discard,
  });

  return (
    <div className="flex gap-2 items-center">
      <IconKey className="h-4 w-4 text-muted-foreground shrink-0" />
      <div className="relative flex-1">
        <Input
          type={showToken ? "text" : "password"}
          placeholder="glpat-xxxxxxxxxxxxxxxxxxxx"
          value={token}
          data-settings-dirty={isDirty}
          onChange={(e) => setToken(e.target.value)}
          className="font-mono text-sm pr-9"
          autoComplete="off"
        />
        <button
          type="button"
          onClick={() => setShowToken((v) => !v)}
          className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground cursor-pointer"
          aria-label={showToken ? "Hide token" : "Show token"}
        >
          {showToken ? <IconEyeOff className="h-4 w-4" /> : <IconEye className="h-4 w-4" />}
        </button>
      </div>
    </div>
  );
}

function ClearTokenButton({ onCleared }: { onCleared: () => void }) {
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
          await clearGitLabToken();
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
      {(ws) => <GitLabConnectionSection key={ws} />}
    </WorkspaceScopedSection>
  );
}

function GitLabConnectionSection() {
  const [status, setStatus] = useState<GitLabStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [hostDirty, setHostDirty] = useState(false);
  const [tokenDirty, setTokenDirty] = useState(false);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      const next = await fetchGitLabStatus({ cache: "no-store" });
      setStatus(next);
    } catch {
      setStatus(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

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
          <IconRefresh className="h-3 w-3" />
          Refresh
        </Button>
      }
    >
      <SettingsCard isDirty={hostDirty || tokenDirty}>
        <CardContent className="space-y-4 py-4">
          <ConnectionErrorAlert status={status} />
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <StatusBadge status={status} />
              {status && <AuthMethodBadge method={status.auth_method} />}
              {status?.glab_version && (
                <Badge variant="outline" className="font-mono text-xs">
                  glab {status.glab_version}
                </Badge>
              )}
            </div>
            {status?.username && (
              <span className="text-xs text-muted-foreground">
                Logged in as <span className="font-medium">{status.username}</span>
              </span>
            )}
          </div>

          <Separator />

          <div className="space-y-2">
            <p className="text-xs text-muted-foreground">
              GitLab host URL. Override for self-managed instances; leave at the default for
              gitlab.com.
            </p>
            <HostForm
              initial={status?.host ?? DEFAULT_HOST}
              onSaved={() => void reload()}
              onDirtyChange={setHostDirty}
            />
          </div>

          <Separator />

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <p className="text-xs text-muted-foreground">
                Personal access token. Required scopes: <code>api</code>, <code>read_user</code>.
                Stored encrypted in the kandev secret store.
              </p>
              {status?.token_configured && <ClearTokenButton onCleared={() => void reload()} />}
            </div>
            <TokenForm onSuccess={() => void reload()} onDirtyChange={setTokenDirty} />
          </div>
        </CardContent>
      </SettingsCard>
    </SettingsSection>
  );
}
