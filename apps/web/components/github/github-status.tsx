"use client";

import { useState, useCallback } from "react";
import {
  IconCheck,
  IconChevronDown,
  IconChevronRight,
  IconInfoCircle,
  IconX,
  IconRefresh,
  IconKey,
  IconTrash,
  IconEye,
  IconEyeOff,
  IconTerminal2,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Input } from "@kandev/ui/input";
import { Spinner } from "@kandev/ui/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { useToast } from "@/components/toast-provider";
import { configureGitHubToken, clearGitHubToken } from "@/lib/api/domains/github-api";
import type { AuthDiagnostics } from "@/lib/types/github";
import { GitHubRateLimitDisplay } from "./github-rate-limit";
import { HostShellDialog } from "@/components/settings/host-shell-dialog";

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

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
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
    },
    [token, toast, onSuccess],
  );

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
            {showToken ? (
              <IconEyeOff className="h-3.5 w-3.5" />
            ) : (
              <IconEye className="h-3.5 w-3.5" />
            )}
          </Button>
        </div>
        <Button
          type="submit"
          size="sm"
          disabled={!token.trim() || saving}
          className="cursor-pointer"
        >
          {saving ? <Spinner className="h-4 w-4" /> : <IconKey className="h-4 w-4 mr-1" />}
          Configure
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">
        Create a{" "}
        <a
          href="https://github.com/settings/tokens/new?scopes=repo,read:org&description=Kandev"
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

type StatusForNotConnected = {
  diagnostics?: AuthDiagnostics;
};

function NotConnectedView({
  status,
  refresh,
  ghAuthOpen,
  setGhAuthOpen,
}: {
  status: StatusForNotConnected | null;
  refresh: () => void;
  ghAuthOpen: boolean;
  setGhAuthOpen: (open: boolean) => void;
}) {
  // gh installed → CLI sign-in is the recommended path (browser-driven OAuth,
  // no PAT to manage). When it's missing we drop to PAT-only.
  const ghInstalled = useAppStore((state) =>
    state.availableAgents.tools.some((t) => t.name === "gh" && t.available),
  );
  const [tokenOpen, setTokenOpen] = useState(false);
  const [diagOpen, setDiagOpen] = useState(false);

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <IconX className="h-4 w-4 text-red-500" />
        <span className="text-sm font-medium">Not connected to GitHub</span>
        <Button variant="ghost" size="sm" onClick={refresh} className="cursor-pointer h-6 px-2">
          <IconRefresh className="h-3.5 w-3.5" />
          Refresh
        </Button>
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              className="ml-auto text-muted-foreground hover:text-foreground cursor-help"
              aria-label="Authentication methods"
            >
              <IconInfoCircle className="h-4 w-4" />
            </button>
          </TooltipTrigger>
          <TooltipContent className="max-w-xs">
            <p className="text-xs">
              Kandev also accepts a <code>GITHUB_TOKEN</code> environment variable on startup. The
              UI options below cover the common cases.
            </p>
          </TooltipContent>
        </Tooltip>
      </div>

      {ghInstalled ? (
        <PrimaryCLISignIn onClick={() => setGhAuthOpen(true)} />
      ) : (
        <CLIUnavailableHint />
      )}

      <Collapsible open={tokenOpen} onOpenChange={setTokenOpen}>
        <CollapsibleTrigger asChild>
          <button
            type="button"
            className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground cursor-pointer"
          >
            {tokenOpen ? (
              <IconChevronDown className="h-3 w-3" />
            ) : (
              <IconChevronRight className="h-3 w-3" />
            )}
            {ghInstalled ? "Use a personal access token instead" : "Configure GitHub token"}
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent className="pt-2 space-y-2">
          <TokenConfigForm onSuccess={refresh} />
          <p className="text-xs text-muted-foreground">
            Create a{" "}
            <a
              href="https://github.com/settings/tokens/new?scopes=repo,read:org&description=KanDev"
              target="_blank"
              rel="noopener noreferrer"
              className="underline hover:text-foreground"
            >
              Personal Access Token
            </a>{" "}
            with <code className="bg-muted px-1 rounded">repo</code> and{" "}
            <code className="bg-muted px-1 rounded">read:org</code> scopes.
          </p>
        </CollapsibleContent>
      </Collapsible>

      {status?.diagnostics && (
        <DiagnosticsDisclosure
          diagnostics={status.diagnostics}
          open={diagOpen}
          onOpenChange={setDiagOpen}
        />
      )}

      <HostShellDialog
        open={ghAuthOpen}
        onOpenChange={setGhAuthOpen}
        initialInput={"gh auth login\n"}
        onClose={refresh}
      />
    </div>
  );
}

function DiagnosticsDisclosure({
  diagnostics,
  open,
  onOpenChange,
}: {
  diagnostics: AuthDiagnostics;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Collapsible open={open} onOpenChange={onOpenChange}>
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground cursor-pointer"
        >
          {open ? (
            <IconChevronDown className="h-3 w-3" />
          ) : (
            <IconChevronRight className="h-3 w-3" />
          )}
          Diagnostics (
          <code className="bg-muted px-1 rounded">{diagnostics.command}</code>, exit{" "}
          {diagnostics.exit_code})
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <DiagnosticsOutput diagnostics={diagnostics} />
      </CollapsibleContent>
    </Collapsible>
  );
}

function PrimaryCLISignIn({ onClick }: { onClick: () => void }) {
  return (
    <div className="rounded-md border border-primary/40 bg-primary/5 p-4 space-y-3">
      <div className="space-y-1">
        <p className="text-sm font-medium">Sign in with the GitHub CLI</p>
        <p className="text-xs text-muted-foreground">
          Recommended. Opens a terminal that runs{" "}
          <code className="bg-muted px-1 rounded">gh auth login</code> with browser-based
          authentication.
        </p>
      </div>
      <Button
        size="default"
        onClick={onClick}
        className="cursor-pointer"
        data-testid="github-gh-auth-login"
      >
        <IconTerminal2 className="h-4 w-4 mr-2" />
        Open terminal and sign in
      </Button>
    </div>
  );
}

function CLIUnavailableHint() {
  return (
    <div className="rounded-md border bg-muted/30 p-3 text-xs text-muted-foreground">
      <p>
        The <code className="bg-muted px-1 rounded">gh</code> CLI is not installed on this host - a
        personal access token is the only sign-in option available here. To install it later, see{" "}
        <a
          href="https://cli.github.com"
          target="_blank"
          rel="noopener noreferrer"
          className="underline hover:text-foreground"
        >
          cli.github.com
        </a>
        .
      </p>
    </div>
  );
}

export function GitHubStatusCard() {
  const { status, loaded, loading, refresh } = useGitHubStatus();
  const { toast } = useToast();
  const [clearing, setClearing] = useState(false);
  const [ghAuthOpen, setGhAuthOpen] = useState(false);

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
      <NotConnectedView
        status={status}
        refresh={refresh}
        ghAuthOpen={ghAuthOpen}
        setGhAuthOpen={setGhAuthOpen}
      />
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
      <GitHubRateLimitDisplay info={status.rate_limit} />
    </div>
  );
}
