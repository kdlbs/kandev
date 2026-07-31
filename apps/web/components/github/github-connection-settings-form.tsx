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
import { GitHubAppConnectionPanel } from "./github-app-connection-panel";
import { GitHubAuthMethodList, type GitHubAutomationMethod } from "./github-auth-method-list";
import { GitHubCLIForm } from "./github-cli-form";
import { GitHubPATForm } from "./github-pat-form";
import { GitHubTaskAccessForm } from "./github-task-credentials-section";

function methodForStatus(status: GitHubStatus): GitHubAutomationMethod {
  if (status.automation?.source === "github_app_installation") return "app";
  if (status.automation?.source === "gh_cli") return "cli";
  return "pat";
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "The settings could not be saved";
}

function runSaveOperations({
  workspaceId,
  connectionRequest,
  taskMode,
  saveTask,
}: {
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
        if (!saved) throw new Error("The workspace changed before task access was saved");
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
    setTaskMode(taskAccess.mode);
    setSaving(false);
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
      toast({ description: "GitHub access settings saved", variant: "success" });
      onComplete();
      return;
    }
    const detail = errorMessage(failures[0].reason);
    toast({
      description:
        successes > 0 ? `Some settings were saved, but another change failed: ${detail}` : detail,
      variant: "error",
    });
  }, [
    canSave,
    connectionRequest,
    onComplete,
    onSaved,
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
  const { method, workspaceId, onMethodChange, taskAccess } = props;
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
          Install a GitHub App above to change the workspace connection. Task access can be saved
          after the installation finishes.
        </p>
      )}
      <Button
        type="button"
        disabled={!canSave}
        onClick={save}
        className="h-11 cursor-pointer"
        data-dialog-default-action
      >
        {saving && <Spinner className="mr-2 h-4 w-4" />}
        {saving ? "Saving changes…" : "Save changes"}
      </Button>
    </div>
  );
}
