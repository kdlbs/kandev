"use client";

import { useCallback, useEffect, useState } from "react";
import {
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
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Separator } from "@kandev/ui/separator";
import { Spinner } from "@kandev/ui/spinner";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
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
  return (
    <Badge variant="outline" className="gap-1">
      <IconX className="h-3 w-3" /> Not connected
    </Badge>
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

function HostForm({ initial, onSaved }: { initial: string; onSaved: () => void }) {
  const [host, setHost] = useState(initial);
  const [saving, setSaving] = useState(false);
  const { toast } = useToast();

  useEffect(() => {
    setHost(initial);
  }, [initial]);

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!host.trim()) return;
      setSaving(true);
      try {
        await configureGitLabHost(host.trim());
        toast({ description: "GitLab host updated", variant: "success" });
        onSaved();
      } catch (err) {
        toast({
          description: err instanceof Error ? err.message : "Failed to update host",
          variant: "error",
        });
      } finally {
        setSaving(false);
      }
    },
    [host, toast, onSaved],
  );

  return (
    <form onSubmit={handleSubmit} className="flex gap-2 items-center">
      <IconWorld className="h-4 w-4 text-muted-foreground shrink-0" />
      <Input
        type="url"
        placeholder={DEFAULT_HOST}
        value={host}
        onChange={(e) => setHost(e.target.value)}
        className="font-mono text-sm"
        disabled={saving}
      />
      <Button type="submit" disabled={saving || !host.trim()} className="cursor-pointer">
        {saving ? <Spinner className="h-3 w-3" /> : "Save host"}
      </Button>
    </form>
  );
}

function TokenForm({ onSuccess }: { onSuccess: () => void }) {
  const [token, setToken] = useState("");
  const [showToken, setShowToken] = useState(false);
  const [saving, setSaving] = useState(false);
  const { toast } = useToast();

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!token.trim()) return;
      setSaving(true);
      try {
        await configureGitLabToken(token.trim());
        toast({ description: "GitLab token configured", variant: "success" });
        setToken("");
        onSuccess();
      } catch (err) {
        toast({
          description: err instanceof Error ? err.message : "Failed to save token",
          variant: "error",
        });
      } finally {
        setSaving(false);
      }
    },
    [token, toast, onSuccess],
  );

  return (
    <form onSubmit={handleSubmit} className="flex gap-2 items-center">
      <IconKey className="h-4 w-4 text-muted-foreground shrink-0" />
      <div className="relative flex-1">
        <Input
          type={showToken ? "text" : "password"}
          placeholder="glpat-xxxxxxxxxxxxxxxxxxxx"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          className="font-mono text-sm pr-9"
          disabled={saving}
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
      <Button type="submit" disabled={saving || !token.trim()} className="cursor-pointer">
        {saving ? <Spinner className="h-3 w-3" /> : "Save token"}
      </Button>
    </form>
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

export function GitLabIntegrationPage() {
  const [status, setStatus] = useState<GitLabStatus | null>(null);
  const [loading, setLoading] = useState(true);

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
      <Card>
        <CardContent className="space-y-4 py-4">
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
            <HostForm initial={status?.host ?? DEFAULT_HOST} onSaved={() => void reload()} />
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
            <TokenForm onSuccess={() => void reload()} />
          </div>
        </CardContent>
      </Card>
    </SettingsSection>
  );
}
