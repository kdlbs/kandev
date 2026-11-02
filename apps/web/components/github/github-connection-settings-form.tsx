"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { normalizeGitHubHost } from "@/lib/github/host";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
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
  host,
  cliAccount,
  automation,
}: {
  method: GitHubAutomationMethod;
  currentMethod: GitHubAutomationMethod;
  token: string;
  host: string;
  cliAccount: GitHubCLIAccount | null;
  automation?: GitHubAutomationConnection | null;
}): SetGitHubConnectionRequest | null {
  if (method === "pat" && token.trim()) return { source: "pat", token: token.trim(), host };
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

function invalidConnectionDraft({
  method,
  currentMethod,
  host,
  token,
  cliAccount,
  automation,
}: {
  method: GitHubAutomationMethod;
  currentMethod: GitHubAutomationMethod;
  host: string;
  token: string;
  cliAccount: GitHubCLIAccount | null;
  automation?: GitHubAutomationConnection | null;
}) {
  if (method === "cli") return !cliAccount;
  if (!normalizeGitHubHost(host)) return true;
  if (method === "app") return currentMethod !== "app";
  return (
    !token.trim() && (currentMethod !== "pat" || host !== (automation?.github_host ?? "github.com"))
  );
}

export function useConnectionSettingsDraft({
  status,
  method,
  workspaceId,
  open,
  onSaved,
  onComplete,
  taskAccess,
}: SettingsFormProps) {
  const [host, setHost] = useState(status.automation?.github_host ?? "github.com");
  const [token, setToken] = useState("");
  const [cliAccount, setCLIAccount] = useState<GitHubCLIAccount | null>(null);
  const [taskMode, setTaskMode] = useState<TaskGitCredentialsMode>(taskAccess.mode);
  const [saving, setSaving] = useState(false);
  const scope = `${workspaceId}:${method}:${host}:${open}`;
  const activeScope = useRef({ key: scope });
  if (activeScope.current.key !== scope) activeScope.current = { key: scope };
  useEffect(() => {
    if (!open) return;
    setToken("");
    setHost(status.automation?.github_host ?? "github.com");
    setCLIAccount(null);
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
        host: normalizeGitHubHost(host) ?? host,
        cliAccount,
        automation: status.automation,
      }),
    [cliAccount, currentMethod, host, method, status.automation, token],
  );
  const methodNeedsWorkflow = method === "app" && currentMethod !== "app";
  const connectionInvalid = invalidConnectionDraft({
    method,
    currentMethod,
    host,
    token,
    cliAccount,
    automation: status.automation,
  });
  const canSave =
    !saving &&
    !taskAccess.loading &&
    !taskAccess.error &&
    !connectionInvalid &&
    (Boolean(connectionRequest) || taskDirty);
  const save = useConnectionSave({
    canSave,
    workspaceId,
    activeScope,
    connectionRequest,
    taskMode: taskDirty ? taskMode : null,
    taskAccess,
    setSaving,
    setToken,
    onSaved,
    onComplete,
  });
  return {
    host,
    setHost: (value: string) => {
      setHost(value);
      setToken("");
    },
    cliAccount,
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

function useConnectionSave({
  canSave,
  workspaceId,
  activeScope,
  connectionRequest,
  taskMode,
  taskAccess,
  setSaving,
  setToken,
  onSaved,
  onComplete,
}: {
  canSave: boolean;
  workspaceId: string;
  activeScope: React.MutableRefObject<{ key: string }>;
  connectionRequest: SetGitHubConnectionRequest | null;
  taskMode: TaskGitCredentialsMode | null;
  taskAccess: TaskGitCredentialsState;
  setSaving: (value: boolean) => void;
  setToken: (value: string) => void;
  onSaved: () => void;
  onComplete: () => void;
}) {
  const { t } = useTranslation();
  const { toast } = useToast();
  return useCallback(async () => {
    if (!canSave) return;
    const savingScope = activeScope.current;
    setSaving(true);
    const results = await runSaveOperations({
      t,
      workspaceId,
      connectionRequest,
      taskMode,
      saveTask: taskAccess.save,
    });
    if (activeScope.current !== savingScope) return;
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
    taskMode,
    toast,
    workspaceId,
  ]);
}

export function GitHubConnectionSettingsForm(
  props: SettingsFormProps & { draft: ReturnType<typeof useConnectionSettingsDraft> },
) {
  const { t } = useTranslation();
  const { method, onMethodChange, taskAccess, isMobile } = props;
  const { taskMode, setTaskMode, saving, methodNeedsWorkflow, canSave, save } = props.draft;

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
            <GitHubAuthMethodList value={method} onChange={onMethodChange} disabled={saving} />
            {method !== "cli" && <ConnectionHostFields {...props} />}
            <ConnectionCredentialFields {...props} />
            {method === "cli" && <ConnectionHostFields {...props} />}
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
          className={controlSizingClassName("standard", "w-full cursor-pointer sm:w-auto")}
          data-dialog-default-action
        >
          {saving && <Spinner aria-hidden="true" className="mr-2 h-4 w-4" />}
          {t("github:saveChanges")}
        </Button>
        <span role="status" className="sr-only">
          {saving ? t("github:savingChanges") : ""}
        </span>
      </div>
    </div>
  );
}

type FormWithDraft = SettingsFormProps & { draft: ReturnType<typeof useConnectionSettingsDraft> };

function ConnectionHostFields({ method, status, draft }: FormWithDraft) {
  const { t } = useTranslation();
  const { host, setHost, cliAccount, saving } = draft;
  const canonicalHost = normalizeGitHubHost(host);
  const proposed = method === "cli" ? cliAccount?.host : canonicalHost;
  const current = status.automation?.github_host;
  return (
    <>
      {method !== "cli" && (
        <div className="space-y-2">
          <Label htmlFor="github-host">{t("github:githubHost")}</Label>
          <Input
            id="github-host"
            data-testid="github-host"
            value={host}
            onChange={(event) => setHost(event.target.value)}
            disabled={saving}
            className={controlSizingClassName("standard")}
            aria-invalid={!canonicalHost}
            aria-describedby={!canonicalHost ? "github-host-error" : undefined}
          />
          {!canonicalHost && (
            <p id="github-host-error" role="alert" className="text-xs text-destructive">
              {t("github:invalidGitHubHost")}
            </p>
          )}
        </div>
      )}
      {method === "cli" && cliAccount && (
        <p data-testid="github-selected-host" className="break-all text-xs text-muted-foreground">
          {t("github:selectedGitHubHost", { host: cliAccount.host })}
        </p>
      )}
      {current && proposed && proposed !== current && (
        <p
          data-testid="github-host-replacement"
          className="text-xs leading-5 text-muted-foreground"
        >
          {t("github:githubHostReplacement", { current, proposed })}
        </p>
      )}
    </>
  );
}

function ConnectionCredentialFields({ method, workspaceId, draft }: FormWithDraft) {
  const { host, token, setToken, setCLIAccount, saving } = draft;
  return (
    <div className="border-t pt-5">
      {method === "pat" && (
        <GitHubPATForm
          githubHost={host}
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
      {method === "app" && normalizeGitHubHost(host) && (
        <GitHubAppConnectionPanel
          key={`${workspaceId}:${host}`}
          workspaceId={workspaceId}
          githubHost={normalizeGitHubHost(host) ?? ""}
        />
      )}
    </div>
  );
}
