"use client";

import { IconCheck, IconX, IconRefresh } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
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

export function GitHubStatusCard() {
  const { status, loaded, loading, refresh } = useGitHubStatus();

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
      <div>
        <div className="flex items-center gap-2">
          <IconX className="h-4 w-4 text-red-500" />
          <span className="text-sm">Not connected to GitHub</span>
          <span className="text-xs text-muted-foreground">
            Run <code className="bg-muted px-1 rounded">gh auth login</code> or add a GITHUB_TOKEN
            secret
          </span>
          <Button variant="ghost" size="sm" onClick={refresh} className="cursor-pointer h-6 px-2">
            <IconRefresh className="h-3.5 w-3.5" />
            <span className="sr-only">Refresh GitHub status</span>
          </Button>
        </div>
        {status?.diagnostics && <DiagnosticsOutput diagnostics={status.diagnostics} />}
      </div>
    );
  }

  return (
    <div className="flex items-center gap-2">
      <IconCheck className="h-4 w-4 text-green-500" />
      <span className="text-sm">
        Connected as <strong>{status.username}</strong>
      </span>
      <Badge variant="secondary" className="text-xs">
        {status.auth_method === "gh_cli" ? "gh CLI" : "PAT"}
      </Badge>
    </div>
  );
}
