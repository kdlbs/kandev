"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { useToast } from "@/components/toast-provider";
import type { TaskGitCredentialsState } from "@/hooks/domains/github/use-task-git-credentials";
import {
  setGitHubWorkspaceConnection,
  type SetGitHubConnectionRequest,
} from "@/lib/api/domains/github-api";
import type {
  GitHubAutomationConnection,
  GitHubCLIAccount,
  GitHubStatus,
  TaskGitCredentialsMode,
} from "@/lib/types/github";
import { cn } from "@/lib/utils";
import { GitHubAppConnectionPanel } from "./github-app-connection-panel";
import { GitHubAuthMethodList, type GitHubAutomationMethod } from "./github-auth-method-list";
import { GitHubCLIForm } from "./github-cli-form";
import { GitHubPATForm } from "./github-pat-form";
import { GitHubTaskAccessForm } from "./github-task-credentials-section";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

function methodForStatus(status: GitHubStatus): GitHubAutomationMethod {
  if (status.automation?.source === "github_app_installation") return "app";
  if (status.automation?.source === "gh_cli") return "cli";
  return "pat";
}

// `t` is threaded in: these are plain functions, and their messages are shown
// to the user through the caller's toast.
function errorMessage(t: TFunction, error: unknown) {
  return error instanceof Error ? error.message : t("github:theSettingsCouldNotBeSaved");
}

// A partial save reports which change failed alongside the ones that landed.
function saveFailureToast(t: TFunction, reason: unknown, successes: number) {
  const detail = errorMessage(t, reason);
  return {
    description: successes > 0 ? t("github:someSettingsWereSavedButAnother", { detail }) : detail,
    variant: "error" as const,
  };
}

function runSaveOperations({
  t,
  workspaceId,
  connectionRequest,
  taskMode,
  saveTask,
}: {
  t: TFunction;
  workspaceId: string;
  connectionRequest: SetGitHubConnectionRequest | null;
  taskMode: TaskGitCredentialsMode | null;
  saveTask: TaskGitCredentialsState["save"];
}) {
  const operations: Promise<unknown>[] = [];
  if (connectionRequest) {
    operations.push(setGitHubWorkspaceConnection(workspaceId, connectionRequest));
  }
  if (taskMode) {
    operations.push(
      saveTask(taskMode).then((saved) => {
        if (!saved) throw new Error(t("github:theWorkspaceChangedBeforeTaskAccess"));
      }),
    );
  }
  return Promise.allSettled(operations);
}

function buildConnectionRequest({
  method,
  currentMethod,
  token,
  cliAccount,
  automation,
}: {
  method: GitHubAutomationMethod;
  currentMethod: GitHubAutomationMethod;
  token: string;
  cliAccount: GitHubCLIAccount | null;
  automation?: GitHubAutomationConnection | null;
}): SetGitHubConnectionRequest | null {
  if (method === "pat" && token.trim()) return { source: "pat", token: token.trim() };
  if (
    method === "cli" &&
    cliAccount &&
    (currentMethod !== "cli" ||
      automation?.github_host !== cliAccount.host ||
      automation.login !== cliAccount.login)
  ) {
    return { source: "gh_cli", host: cliAccount.host, login: cliAccount.login };
  }
  return null;
}

type SettingsFormProps = {
  status: GitHubStatus;
  method: GitHubAutomationMethod;
  workspaceId: string;
  open: boolean;
  onMethodChange: (method: GitHubAutomationMethod) => void;
  onSaved: () => void;
  onComplete: () => void;
  taskAccess: TaskGitCredentialsState;
  isMobile: boolean;
};

function useConnectionSettingsDraft({
  status,
  method,
  workspaceId,
  open,
  onSaved,
  onComplete,
  taskAccess,
}: SettingsFormProps) {
  const { t } = useTranslation();
  const [token, setToken] = useState("");
  const [cliAccount, setCLIAccount] = useState<GitHubCLIAccount | null>(null);
  const [taskMode, setTaskMode] = useState<TaskGitCredentialsMode>(taskAccess.mode);
  const [saving, setSaving] = useState(false);
  const activeWorkspaceId = useRef(workspaceId);
  const { toast } = useToast();
  activeWorkspaceId.current = workspaceId;
  useEffect(() => {
    if (!open) return;
    setToken("");
    setSaving(false);
  }, [open, workspaceId]);
  useEffect(() => {
    if (open) setTaskMode(taskAccess.mode);
  }, [open, taskAccess.mode, workspaceId]);
  const currentMethod = methodForStatus(status);
  const taskDirty = taskMode !== taskAccess.mode;
  const connectionRequest = useMemo(
    () =>
      buildConnectionRequest({
        method,
        currentMethod,
        token,
        cliAccount,
        automation: status.automation,
      }),
    [cliAccount, currentMethod, method, status.automation, token],
  );
  const methodNeedsWorkflow = method === "app" && currentMethod !== "app";
  const connectionInvalid =
    (method === "pat" && currentMethod !== "pat" && !token.trim()) ||
    (method === "cli" && !cliAccount) ||
    methodNeedsWorkflow;
  const canSave =
    !saving &&
    !taskAccess.loading &&
    !taskAccess.error &&
    !connectionInvalid &&
    (Boolean(connectionRequest) || taskDirty);
  const save = useCallback(async () => {
    if (!canSave) return;
    const savingWorkspaceId = workspaceId;
    setSaving(true);
    const results = await runSaveOperations({
      t,
      workspaceId,
      connectionRequest,
      taskMode: taskDirty ? taskMode : null,
      saveTask: taskAccess.save,
    });
    if (activeWorkspaceId.current !== savingWorkspaceId) return;
    setSaving(false);
    const failures = results.filter(
      (result): result is PromiseRejectedResult => result.status === "rejected",
    );
    const successes = results.length - failures.length;
    if (successes > 0) onSaved();
    if (failures.length === 0) {
      setToken("");
      toast({ description: t("github:githubAccessSettingsSaved"), variant: "success" });
      onComplete();
      return;
    }
    toast(saveFailureToast(t, failures[0].reason, successes));
  }, [
    canSave,
    connectionRequest,
    onComplete,
    onSaved,
    t,
    taskAccess,
    taskDirty,
    taskMode,
    toast,
    workspaceId,
  ]);
  return {
    token,
    setToken,
    setCLIAccount,
    taskMode,
    setTaskMode,
    saving,
    methodNeedsWorkflow,
    canSave,
    save,
  };
}

export function GitHubConnectionSettingsForm(props: SettingsFormProps) {
  const { t } = useTranslation();
  const { method, workspaceId, onMethodChange, taskAccess, isMobile } = props;
  const {
    token,
    setToken,
    setCLIAccount,
    taskMode,
    setTaskMode,
    saving,
    methodNeedsWorkflow,
    canSave,
    save,
  } = useConnectionSettingsDraft(props);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="relative flex min-h-0 flex-1 flex-col overflow-hidden">
        <div
          data-testid="github-connection-scroll"
          className={cn(
            "min-h-0 flex-1 overflow-y-auto overscroll-contain pb-8",
            isMobile ? "px-4 pt-4" : "pr-1",
          )}
        >
          <div className="space-y-5">
            <GitHubAuthMethodList value={method} onChange={onMethodChange} />
            <div className="border-t pt-5">
              {method === "pat" && (
                <GitHubPATForm
                  workspaceId={workspaceId}
                  value={token}
                  onChange={setToken}
                  disabled={saving}
                />
              )}
              {method === "cli" && (
                <GitHubCLIForm
                  workspaceId={workspaceId}
                  onAccountChange={setCLIAccount}
                  disabled={saving}
                />
              )}
              {method === "app" && <GitHubAppConnectionPanel workspaceId={workspaceId} />}
            </div>
            <GitHubTaskAccessForm
              taskAccess={taskAccess}
              value={taskMode}
              onChange={setTaskMode}
              disabled={saving}
            />
            {methodNeedsWorkflow && (
              <p className="text-xs leading-5 text-muted-foreground">
                {t("github:installAGithubAppAboveTo")}
              </p>
            )}
          </div>
        </div>
        <div
          data-testid="github-connection-scroll-fade"
          aria-hidden="true"
          className="pointer-events-none absolute inset-x-0 bottom-0 h-6 bg-gradient-to-t from-popover to-transparent"
        />
      </div>
      <div
        data-testid="github-connection-footer"
        className={cn(
          "flex shrink-0 justify-end border-t bg-popover pt-3",
          isMobile ? "px-4 pb-[calc(0.75rem+env(safe-area-inset-bottom,0px))]" : "pb-0",
        )}
      >
        <Button
          type="button"
          disabled={!canSave}
          onClick={save}
          className="h-11 w-full cursor-pointer sm:w-auto"
          data-dialog-default-action
        >
          {saving && <Spinner className="mr-2 h-4 w-4" />}
          {saving ? t("github:savingChanges") : t("github:saveChanges")}
        </Button>
      </div>
    </div>
  );
}
