"use client";

import { useState, useCallback } from "react";
import { IconCheck, IconX, IconRefresh, IconKey, IconTrash, IconEye, IconEyeOff } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Spinner } from "@kandev/ui/spinner";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { useToast } from "@/components/toast-provider";
import { configureGitHubToken, clearGitHubToken } from "@/lib/api/domains/github-api";
import type { AuthDiagnostics } from "@/lib/types/github";

function DiagnosticsOutput({ diagnostics }: { diagnostics: AuthDiagnostics }) {
  return (
    <div className="mt-3 space-y-2">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <code className="bg-muted px-1.5 py-0.5 rounded">{diagnostics.command}</code>
        <Badge
          variant={diagnostics.exit_code === 0 ? "secondary" : "destructive"}
          className="text-xs"
        >
          exit code: {diagnostics.exit_code}
        </Badge>
      </div>
      {diagnostics.exit_code !== 0 && (
        <p className="text-xs text-muted-foreground">
          A non-zero exit code means the command failed. Review the output below for details.
        </p>
      )}
      <pre className="text-xs bg-muted/50 border rounded-md p-3 overflow-x-auto whitespace-pre-wrap max-h-48">
        {diagnostics.output.trim()}
      </pre>
    </div>
  );
}

function TokenConfigForm({ onSuccess }: { onSuccess: () => void }) {
  const [token, setToken] = useState("");
  const [showToken, setShowToken] = useState(false);
  const [saving, setSaving] = useState(false);
  const { toast } = useToast();

  const handleSubmit = useCallback(async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token.trim()) return;

    setSaving(true);
    try {
      await configureGitHubToken(token.trim());
      toast({ description: "GitHub token configured successfully", variant: "success" });
      setToken("");
      onSuccess();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to configure token";
      toast({ description: message, variant: "error" });
    } finally {
      setSaving(false);
    }
  }, [token, toast, onSuccess]);

  return (
    <form onSubmit={handleSubmit} className="space-y-3">
      <div className="flex gap-2">
        <div className="relative flex-1">
          <Input
            type={showToken ? "text" : "password"}
            placeholder="ghp_xxxxxxxxxxxx"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            className="pr-8 font-mono text-sm"
          />
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="absolute right-1 top-1/2 -translate-y-1/2 h-6 w-6 p-0 cursor-pointer"
            onClick={() => setShowToken(!showToken)}
          >
            {showToken ? <IconEyeOff className="h-3.5 w-3.5" /> : <IconEye className="h-3.5 w-3.5" />}
          </Button>
        </div>
        <Button type="submit" size="sm" disabled={!token.trim() || saving} className="cursor-pointer">
          {saving ? <Spinner className="h-4 w-4" /> : <IconKey className="h-4 w-4 mr-1" />}
          Configure
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">
        Create a{" "}
        <a
          href="https://github.com/settings/tokens/new?scopes=repo,read:org&description=KanDev"
          target="_blank"
          rel="noopener noreferrer"
          className="underline cursor-pointer"
        >
          Personal Access Token
        </a>{" "}
        with <code className="bg-muted px-1 rounded">repo</code> and{" "}
        <code className="bg-muted px-1 rounded">read:org</code> scopes.
      </p>
    </form>
  );
}

export function GitHubStatusCard() {
  const { status, loaded, loading, refresh } = useGitHubStatus();
  const { toast } = useToast();
  const [clearing, setClearing] = useState(false);

  const handleClearToken = useCallback(async () => {
    setClearing(true);
    try {
      await clearGitHubToken();
      toast({ description: "GitHub token removed", variant: "success" });
      refresh();
    } catch {
      toast({ description: "Failed to clear token", variant: "error" });
    } finally {
      setClearing(false);
    }
  }, [toast, refresh]);

  if (loading || !loaded) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Spinner className="h-4 w-4" />
        Checking GitHub connection...
      </div>
    );
  }

  if (!status || !status.authenticated) {
    return (
      <div className="space-y-4">
        <div className="flex items-center gap-2">
          <IconX className="h-4 w-4 text-red-500" />
          <span className="text-sm font-medium">Not connected to GitHub</span>
          <Button variant="ghost" size="sm" onClick={refresh} className="cursor-pointer h-6 px-2">
            <IconRefresh className="h-3.5 w-3.5" />
            Refresh
          </Button>
        </div>

        {/* Token configuration form */}
        <div className="space-y-2">
          <p className="text-sm font-medium">Configure GitHub Token</p>
          <TokenConfigForm onSuccess={refresh} />
        </div>

        {/* Alternative methods */}
        <div className="text-xs text-muted-foreground space-y-1.5 border-t pt-3">
          <p>Other authentication methods:</p>
          <ul className="list-disc list-inside space-y-1 pl-1">
            <li>
              <strong>Environment variable</strong> — set{" "}
              <code className="bg-muted px-1 rounded">GITHUB_TOKEN</code> when starting Kandev
            </li>
            <li>
              <strong>GitHub CLI</strong> — run{" "}
              <code className="bg-muted px-1 rounded">gh auth login</code>
            </li>
          </ul>
        </div>
        {status?.diagnostics && <DiagnosticsOutput diagnostics={status.diagnostics} />}
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <IconCheck className="h-4 w-4 text-green-500" />
        <span className="text-sm">
          Connected as <strong>{status.username}</strong>
        </span>
        <Badge variant="secondary" className="text-xs">
          {status.auth_method === "gh_cli" ? "gh CLI" : "PAT"}
        </Badge>
        {status.token_configured && (
          <Button
            variant="ghost"
            size="sm"
            onClick={handleClearToken}
            disabled={clearing}
            className="cursor-pointer h-6 px-2 text-muted-foreground hover:text-destructive"
            title="Remove configured token"
          >
            {clearing ? <Spinner className="h-3.5 w-3.5" /> : <IconTrash className="h-3.5 w-3.5" />}
          </Button>
        )}
      </div>
      {status.token_configured && (
        <p className="text-xs text-muted-foreground">
          Token stored in secrets. This token is used by remote agents for GitHub operations.
        </p>
      )}
    </div>
  );
}
