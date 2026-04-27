"use client";

import { useCallback, useEffect, useState } from "react";
import {
  IconGitBranch,
  IconRefresh,
  IconArrowUp,
  IconArrowDown,
  IconCheck,
  IconAlertTriangle,
} from "@tabler/icons-react";
import { Input } from "@kandev/ui/input";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { toast } from "sonner";
import { useAppStore } from "@/components/state-provider";
import * as orchestrateApi from "@/lib/api/domains/orchestrate-api";
import type { GitStatusData } from "@/lib/api/domains/orchestrate-api";

function useGitOperations(activeWorkspaceId: string) {
  const [gitStatus, setGitStatus] = useState<GitStatusData | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchStatus = useCallback(async () => {
    if (!activeWorkspaceId) return;
    try {
      const status = await orchestrateApi.getGitStatus(activeWorkspaceId);
      setGitStatus(status);
      setError(null);
    } catch {
      setGitStatus(null);
    }
  }, [activeWorkspaceId]);

  useEffect(() => {
    fetchStatus();
  }, [fetchStatus]);

  const runOp = useCallback(
    async (op: () => Promise<unknown>, successMsg: string, failPrefix: string) => {
      if (!activeWorkspaceId) return;
      setLoading(true);
      setError(null);
      try {
        await op();
        toast.success(successMsg);
        await fetchStatus();
      } catch (err) {
        const msg = err instanceof Error ? err.message : `${failPrefix} failed`;
        setError(msg);
        toast.error(msg);
      } finally {
        setLoading(false);
      }
    },
    [activeWorkspaceId, fetchStatus],
  );

  return { gitStatus, loading, error, fetchStatus, runOp };
}

export function GitSection() {
  const activeWorkspaceId = useAppStore((s) => s.workspaces?.activeId ?? "");
  const [repoUrl, setRepoUrl] = useState("");
  const [branch, setBranch] = useState("main");
  const [commitMessage, setCommitMessage] = useState("");
  const { gitStatus, loading, error, fetchStatus, runOp } = useGitOperations(activeWorkspaceId);

  const handleClone = useCallback(
    () =>
      runOp(
        async () => {
          await orchestrateApi.gitClone(activeWorkspaceId, {
            repoUrl,
            branch,
            workspaceName: activeWorkspaceId,
          });
        },
        "Repository cloned",
        "Clone",
      ),
    [activeWorkspaceId, repoUrl, branch, runOp],
  );

  const handlePull = useCallback(
    () => runOp(() => orchestrateApi.gitPull(activeWorkspaceId), "Pulled latest changes", "Pull"),
    [activeWorkspaceId, runOp],
  );

  const handlePush = useCallback(
    () =>
      runOp(
        async () => {
          await orchestrateApi.gitPush(activeWorkspaceId, {
            message: commitMessage || "Update workspace configuration",
          });
          setCommitMessage("");
        },
        "Changes pushed",
        "Push",
      ),
    [activeWorkspaceId, commitMessage, runOp],
  );

  const isGit = gitStatus?.is_git ?? false;

  return (
    <section className="space-y-4">
      {error && <ErrorBanner message={error} />}

      {!isGit && (
        <CloneForm
          repoUrl={repoUrl}
          branch={branch}
          loading={loading}
          onRepoUrlChange={setRepoUrl}
          onBranchChange={setBranch}
          onClone={handleClone}
        />
      )}

      {isGit && gitStatus && (
        <GitStatusDisplay
          status={gitStatus}
          commitMessage={commitMessage}
          loading={loading}
          onCommitMessageChange={setCommitMessage}
          onPull={handlePull}
          onPush={handlePush}
          onRefresh={fetchStatus}
        />
      )}
    </section>
  );
}

function ErrorBanner({ message }: { message: string }) {
  return (
    <div className="flex items-start gap-2 rounded-md bg-destructive/10 border border-destructive/20 p-3">
      <IconAlertTriangle className="h-4 w-4 text-destructive mt-0.5 shrink-0" />
      <p className="text-xs text-destructive">{message}</p>
    </div>
  );
}

function CloneForm({
  repoUrl,
  branch,
  loading,
  onRepoUrlChange,
  onBranchChange,
  onClone,
}: {
  repoUrl: string;
  branch: string;
  loading: boolean;
  onRepoUrlChange: (v: string) => void;
  onBranchChange: (v: string) => void;
  onClone: () => void;
}) {
  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">
        Connect a git repository to version-control this workspace configuration.
      </p>
      <div>
        <label className="text-sm text-muted-foreground">Repository URL</label>
        <Input
          value={repoUrl}
          onChange={(e) => onRepoUrlChange(e.target.value)}
          placeholder="https://github.com/org/config.git"
          className="mt-1"
        />
      </div>
      <div>
        <label className="text-sm text-muted-foreground">Branch</label>
        <Input
          value={branch}
          onChange={(e) => onBranchChange(e.target.value)}
          placeholder="main"
          className="mt-1"
        />
      </div>
      <Button onClick={onClone} disabled={loading || !repoUrl} className="cursor-pointer">
        <IconGitBranch className="h-4 w-4 mr-1" />
        {loading ? "Cloning..." : "Clone"}
      </Button>
    </div>
  );
}

function GitStatusDisplay({
  status,
  commitMessage,
  loading,
  onCommitMessageChange,
  onPull,
  onPush,
  onRefresh,
}: {
  status: GitStatusData;
  commitMessage: string;
  loading: boolean;
  onCommitMessageChange: (v: string) => void;
  onPull: () => void;
  onPush: () => void;
  onRefresh: () => void;
}) {
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <IconGitBranch className="h-4 w-4 text-muted-foreground" />
          <span className="text-sm font-mono">{status.branch}</span>
          {status.is_dirty ? (
            <Badge variant="outline" className="text-yellow-600 border-yellow-300 text-[10px]">
              dirty
            </Badge>
          ) : (
            <Badge variant="outline" className="text-green-600 border-green-300 text-[10px]">
              <IconCheck className="h-3 w-3 mr-0.5" />
              clean
            </Badge>
          )}
        </div>
        <Button variant="ghost" size="sm" onClick={onRefresh} className="cursor-pointer">
          <IconRefresh className="h-3.5 w-3.5" />
        </Button>
      </div>

      {status.has_remote && (status.ahead > 0 || status.behind > 0) && (
        <div className="flex gap-3 text-xs text-muted-foreground">
          {status.ahead > 0 && (
            <span className="flex items-center gap-1">
              <IconArrowUp className="h-3 w-3" />
              {status.ahead} ahead
            </span>
          )}
          {status.behind > 0 && (
            <span className="flex items-center gap-1">
              <IconArrowDown className="h-3 w-3" />
              {status.behind} behind
            </span>
          )}
        </div>
      )}

      <div className="flex gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={onPull}
          disabled={loading}
          className="cursor-pointer"
        >
          <IconArrowDown className="h-3.5 w-3.5 mr-1" />
          {loading ? "Pulling..." : "Pull"}
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={onPush}
          disabled={loading}
          className="cursor-pointer"
        >
          <IconArrowUp className="h-3.5 w-3.5 mr-1" />
          {loading ? "Pushing..." : "Push"}
        </Button>
      </div>

      <div>
        <label className="text-sm text-muted-foreground">Commit message</label>
        <Input
          value={commitMessage}
          onChange={(e) => onCommitMessageChange(e.target.value)}
          placeholder="Update workspace configuration"
          className="mt-1"
        />
      </div>
    </div>
  );
}
