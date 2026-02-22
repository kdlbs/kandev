"use client";

import { IconCheck, IconX } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Spinner } from "@kandev/ui/spinner";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";

export function GitHubStatusCard() {
  const { status, loaded, loading } = useGitHubStatus();

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
      <div className="flex items-center gap-2">
        <IconX className="h-4 w-4 text-red-500" />
        <span className="text-sm">Not connected to GitHub</span>
        <span className="text-xs text-muted-foreground">
          Run <code className="bg-muted px-1 rounded">gh auth login</code> or add a GITHUB_TOKEN
          secret
        </span>
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
