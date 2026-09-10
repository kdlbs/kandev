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
import { toast } from "@/lib/toast/sonner";
import { useAppStore } from "@/components/state-provider";
import * as officeApi from "@/lib/api/domains/office-api";
import type { GitStatusData } from "@/lib/api/domains/office-api";
import { useOfficeConfigSyncActive } from "@/hooks/domains/office/use-office-config-sync-active";
import { useTranslation } from "react-i18next";

function useGitOperations(activeWorkspaceId: string) {
  const { t } = useTranslation();
  const [gitStatus, setGitStatus] = useState<GitStatusData | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchStatus = useCallback(async () => {
    if (!activeWorkspaceId) return;
    try {
      const status = await officeApi.getGitStatus(activeWorkspaceId);
      setGitStatus(status);
      setError(null);
    } catch {
      setGitStatus(null);
    }
  }, [activeWorkspaceId]);

  useEffect(() => {
    fetchStatus();
  }, [fetchStatus]);

  // Catalog KEYS, not messages: `failureKey` used to be a verb ("Clone") dropped
  // into a `${verb} failed` frame, which no language can reorder or inflect.
  // Each operation now carries a whole failure sentence of its own.
  const runOp = useCallback(
    async (op: () => Promise<unknown>, successKey: string, failureKey: string) => {
      if (!activeWorkspaceId) return;
      setLoading(true);
      setError(null);
      try {
        await op();
        toast.success(t(successKey));
        await fetchStatus();
      } catch (err) {
        const msg = err instanceof Error ? err.message : t(failureKey);
        setError(msg);
        toast.error(msg);
      } finally {
        setLoading(false);
      }
    },
    [activeWorkspaceId, fetchStatus, t],
  );

  return { gitStatus, loading, error, fetchStatus, runOp };
}

export function GitSection() {
  const activeWorkspaceId = useAppStore((s) => s.workspaces?.activeId ?? "");
  const [repoUrl, setRepoUrl] = useState("");
  const [branch, setBranch] = useState("main");
  const [commitMessage, setCommitMessage] = useState("");
  const { gitStatus, loading, error, fetchStatus, runOp } = useGitOperations(activeWorkspaceId);
  // AC-OFFICE-CONFIG-SYNC-006.6: clone/pull are refused server-side while
  // config sync owns this workspace's configuration; push stays available
  // since it is never refused (AC-OFFICE-CONFIG-SYNC-005.5).
  const configSyncActive = useOfficeConfigSyncActive(activeWorkspaceId);

  const handleClone = useCallback(
    () =>
      runOp(
        async () => {
          await officeApi.gitClone(activeWorkspaceId, {
            repoUrl,
            branch,
            workspaceName: activeWorkspaceId,
          });
        },
        "office:repositoryCloned",
        "office:cloneFailed",
      ),
    [activeWorkspaceId, repoUrl, branch, runOp],
  );

  const handlePull = useCallback(
    () =>
      runOp(
        () => officeApi.gitPull(activeWorkspaceId),
        "office:pulledLatestChanges",
        "office:pullFailed",
      ),
    [activeWorkspaceId, runOp],
  );

  const handlePush = useCallback(
    () =>
      runOp(
        async () => {
          // i18n-exempt: becomes the git commit message. See the comment below.
          await officeApi.gitPush(activeWorkspaceId, {
            // Literal: this becomes the git COMMIT MESSAGE, so it is persisted
            // content rather than UI copy. The input placeholder that shows it
            // is translated; the value written to history stays English.
            message: commitMessage || "Update workspace configuration",
          });
          setCommitMessage("");
        },
        "office:changesPushed",
        "office:pushFailed",
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
          disabled={configSyncActive}
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
          pullDisabled={configSyncActive}
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
  disabled,
  onRepoUrlChange,
  onBranchChange,
  onClone,
}: {
  repoUrl: string;
  branch: string;
  loading: boolean;
  disabled: boolean;
  onRepoUrlChange: (v: string) => void;
  onBranchChange: (v: string) => void;
  onClone: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">{t("office:connectAGitRepositoryToVersion")}</p>
      <div>
        <label className="text-sm text-muted-foreground">{t("office:repositoryUrl")}</label>
        <Input
          value={repoUrl}
          onChange={(e) => onRepoUrlChange(e.target.value)}
          placeholder="https://github.com/org/config.git"
          className="mt-1"
        />
      </div>
      <div>
        <label className="text-sm text-muted-foreground">{t("office:branch")}</label>
        <Input
          value={branch}
          onChange={(e) => onBranchChange(e.target.value)}
          placeholder={t("office:main")}
          className="mt-1"
        />
      </div>
      <Button
        onClick={onClone}
        disabled={loading || !repoUrl || disabled}
        className="cursor-pointer"
        data-testid="office-git-clone"
      >
        <IconGitBranch className="h-4 w-4 mr-1" />
        {loading ? t("office:cloning") : t("office:clone")}
      </Button>
      {disabled && (
        <p className="text-xs text-muted-foreground" data-testid="office-git-clone-disabled-reason">
          {t("office:configSyncActiveGuardReason")}
        </p>
      )}
    </div>
  );
}

function GitStatusDisplay({
  status,
  commitMessage,
  loading,
  pullDisabled,
  onCommitMessageChange,
  onPull,
  onPush,
  onRefresh,
}: {
  status: GitStatusData;
  commitMessage: string;
  loading: boolean;
  pullDisabled: boolean;
  onCommitMessageChange: (v: string) => void;
  onPull: () => void;
  onPush: () => void;
  onRefresh: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <IconGitBranch className="h-4 w-4 text-muted-foreground" />
          <span className="text-sm font-mono">{status.branch}</span>
          {status.is_dirty ? (
            <Badge variant="outline" className="text-yellow-600 border-yellow-300 text-[10px]">
              {t("office:dirty")}
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
          disabled={loading || pullDisabled}
          className="cursor-pointer"
          data-testid="office-git-pull"
        >
          <IconArrowDown className="h-3.5 w-3.5 mr-1" />
          {loading ? t("office:pulling") : t("common:commandPreviewPull")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={onPush}
          disabled={loading}
          className="cursor-pointer"
        >
          <IconArrowUp className="h-3.5 w-3.5 mr-1" />
          {loading ? t("office:pushing") : t("office:push")}
        </Button>
      </div>
      {pullDisabled && (
        <p className="text-xs text-muted-foreground" data-testid="office-git-pull-disabled-reason">
          {t("office:configSyncActiveGuardReason")}
        </p>
      )}

      <div>
        <label className="text-sm text-muted-foreground">{t("office:commitMessage")}</label>
        <Input
          value={commitMessage}
          onChange={(e) => onCommitMessageChange(e.target.value)}
          placeholder={t("office:updateWorkspaceConfiguration")}
          className="mt-1"
        />
      </div>
    </div>
  );
}
