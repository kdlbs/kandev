"use client";

import { DialogTitle } from "@kandev/ui/dialog";
import { RepositorySelector, InlineTaskName } from "@/components/task-create-dialog-selectors";
import type { useRepositoryOptions } from "@/components/task-create-dialog-options";

function getRepositoryPlaceholder(
  workspaceId: string | null,
  repositoriesLoading: boolean,
  discoverReposLoading: boolean,
) {
  if (!workspaceId) return "Select workspace first";
  if (repositoriesLoading || discoverReposLoading) return "Loading...";
  return "Select repository";
}

export type DialogHeaderContentProps = {
  isCreateMode: boolean;
  isEditMode: boolean;
  isTaskStarted: boolean;
  sessionRepoName?: string;
  initialTitle?: string;
  taskName: string;
  repositoryId: string;
  discoveredRepoPath: string;
  workspaceId: string | null;
  repositoriesLoading: boolean;
  discoverReposLoading: boolean;
  headerRepositoryOptions: ReturnType<typeof useRepositoryOptions>["headerRepositoryOptions"];
  onRepositoryChange: (v: string) => void;
  onTaskNameChange: (v: string) => void;
  useGitHubUrl: boolean;
  githubUrl: string;
  githubUrlError: string | null;
  onToggleGitHubUrl: () => void;
  onGitHubUrlChange: (v: string) => void;
  repositoryLocked?: boolean;
};

type RepoSourceInputProps = Pick<
  DialogHeaderContentProps,
  | "useGitHubUrl"
  | "githubUrl"
  | "githubUrlError"
  | "onGitHubUrlChange"
  | "isTaskStarted"
  | "headerRepositoryOptions"
  | "repositoryId"
  | "discoveredRepoPath"
  | "onRepositoryChange"
  | "workspaceId"
  | "repositoriesLoading"
  | "discoverReposLoading"
  | "repositoryLocked"
>;

function GhUrlInput({
  githubUrl,
  githubUrlError,
  onGitHubUrlChange,
  isTaskStarted,
}: Pick<
  RepoSourceInputProps,
  "githubUrl" | "githubUrlError" | "onGitHubUrlChange" | "isTaskStarted"
>): React.ReactNode {
  return (
    <div className="relative">
      <input
        type="text"
        value={githubUrl}
        onChange={(e) => onGitHubUrlChange(e.target.value)}
        placeholder="github.com/owner/repo"
        data-testid="github-url-input"
        aria-label="GitHub repository URL"
        aria-invalid={!!githubUrlError}
        aria-describedby={githubUrlError ? "github-url-error" : undefined}
        size={Math.max((githubUrl || "").length + 1, "github.com/owner/repo".length)}
        className={`bg-transparent border-none outline-none focus:ring-0 text-sm font-medium min-w-0 h-7 rounded-md px-2 hover:bg-muted focus:bg-muted transition-colors placeholder:text-muted-foreground ${githubUrlError ? "text-destructive" : ""}`}
        disabled={isTaskStarted}
        autoFocus
      />
      {githubUrlError && (
        <div
          id="github-url-error"
          role="alert"
          className="absolute left-0 top-full mt-1 z-50 rounded-md border bg-popover px-2 py-1 text-[11px] text-destructive shadow-md whitespace-nowrap"
          data-testid="github-url-error"
        >
          {githubUrlError}
        </div>
      )}
    </div>
  );
}

function resolveSelectorPlaceholder(p: RepoSourceInputProps): string {
  if (p.repositoryLocked && !(p.repositoryId || p.discoveredRepoPath)) {
    return "Preparing kandev repository...";
  }
  return getRepositoryPlaceholder(p.workspaceId, p.repositoriesLoading, p.discoverReposLoading);
}

function RepoSourceInput(p: RepoSourceInputProps): React.ReactNode {
  // When the dialog locks the repository (e.g. Improve Kandev pinning kdlbs/kandev),
  // never expose the editable GitHub URL input — even if useGitHubUrl is on.
  if (p.useGitHubUrl && !p.repositoryLocked) return <GhUrlInput {...p} />;
  const isDisabled =
    p.repositoryLocked ||
    p.isTaskStarted ||
    !p.workspaceId ||
    p.repositoriesLoading ||
    p.discoverReposLoading;
  const emptyMessage =
    p.repositoriesLoading || p.discoverReposLoading
      ? "Loading repositories..."
      : "No repositories found.";
  return (
    <RepositorySelector
      options={p.headerRepositoryOptions}
      value={p.repositoryId || p.discoveredRepoPath}
      onValueChange={p.onRepositoryChange}
      placeholder={resolveSelectorPlaceholder(p)}
      searchPlaceholder="Search repositories..."
      emptyMessage={emptyMessage}
      disabled={isDisabled}
      triggerClassName="w-auto text-sm"
    />
  );
}

export function DialogHeaderContent(props: DialogHeaderContentProps) {
  const { isCreateMode, isEditMode, isTaskStarted, sessionRepoName, initialTitle } = props;
  const { taskName, onTaskNameChange, useGitHubUrl, onToggleGitHubUrl, repositoryLocked } = props;

  if (isCreateMode || isEditMode) {
    return (
      <>
        <DialogTitle asChild>
          <div className="flex items-center gap-1 text-sm font-medium min-w-0">
            <RepoSourceInput {...props} />
            <span className="text-muted-foreground mr-2">/</span>
            <InlineTaskName
              value={taskName}
              onChange={onTaskNameChange}
              autoFocus={!isEditMode && !useGitHubUrl}
            />
          </div>
        </DialogTitle>
        {!isTaskStarted && !repositoryLocked && (
          <div className="flex items-center gap-2 pl-2">
            <button
              type="button"
              onClick={onToggleGitHubUrl}
              className="text-xs text-muted-foreground hover:text-foreground cursor-pointer transition-colors"
              data-testid="toggle-github-url"
            >
              {useGitHubUrl ? "or select a repository" : "or paste a GitHub URL"}
            </button>
          </div>
        )}
      </>
    );
  }
  return (
    <DialogTitle asChild>
      <div className="flex items-center gap-1 min-w-0 text-sm font-medium">
        {sessionRepoName && (
          <>
            <span className="truncate text-muted-foreground">{sessionRepoName}</span>
            <span className="text-muted-foreground mx-0.5">/</span>
          </>
        )}
        <span className="truncate">{initialTitle || "Task"}</span>
        <span className="text-muted-foreground mx-0.5">/</span>
        <span className="text-muted-foreground whitespace-nowrap">new session</span>
      </div>
    </DialogTitle>
  );
}
