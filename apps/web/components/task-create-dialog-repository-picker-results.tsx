import { IconCheck } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import type { RemoteRepository } from "@/hooks/domains/integrations/use-remote-repositories";
import type { LocalRepositoryChoice } from "@/components/task-create-dialog-repository-picker";
import type { RepositoryCloneSourceState } from "@/hooks/domains/repositories/use-repository-clone-source";
import { RemoteRepositoryProviderIcon } from "@/components/task-create-dialog-remote-repo-provider-tabs";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

export type RepositoryPickerLocalChoice = {
  key: string;
  label: string;
  path: string;
  choice: LocalRepositoryChoice;
};

export function LocalChoiceList({
  choices,
  cloneSourceStates,
  remoteOriginMode = false,
  onSelect,
}: {
  choices: RepositoryPickerLocalChoice[];
  cloneSourceStates: Record<string, RepositoryCloneSourceState>;
  remoteOriginMode?: boolean;
  onSelect: (choice: LocalRepositoryChoice) => void;
}) {
  const { t } = useTranslation();
  if (choices.length === 0) {
    return <EmptyPickerMessage message={t("task:noRepositoriesFound")} />;
  }
  return (
    <div>
      {choices.map((choice) => (
        <LocalChoiceButton
          key={choice.key}
          choice={choice}
          state={cloneSourceStates[choice.key]}
          remoteOriginMode={remoteOriginMode}
          onSelect={onSelect}
        />
      ))}
    </div>
  );
}

function LocalChoiceButton({
  choice,
  state,
  remoteOriginMode,
  onSelect,
}: {
  choice: RepositoryPickerLocalChoice;
  state?: RepositoryCloneSourceState;
  remoteOriginMode: boolean;
  onSelect: (choice: LocalRepositoryChoice) => void;
}) {
  const { t } = useTranslation();
  const disabled = remoteOriginMode && state?.status !== "ready";
  const cloneSource = state?.status === "ready" ? state.result : undefined;
  const selectedChoice = cloneSource
    ? {
        ...choice.choice,
        defaultBranch: cloneSource.default_branch || choice.choice.defaultBranch,
        checkoutSource: "remote_origin" as const,
        expectedOrigin: cloneSource.origin,
        remoteBranches: cloneSource.branches,
      }
    : choice.choice;
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={() => onSelect(selectedChoice)}
      data-testid="task-repository-local-option"
      className={cn(
        "flex min-h-11 w-full items-center justify-between gap-2 rounded-sm px-2 py-1.5 text-left text-xs sm:min-h-8",
        disabled ? "cursor-not-allowed opacity-50" : "cursor-pointer hover:bg-muted",
      )}
    >
      <span className="flex min-w-0 flex-col">
        <span className="truncate">{choice.label}</span>
        {localChoiceDescription(t, choice, state, remoteOriginMode)}
      </span>
      <IconCheck className="size-4 shrink-0 opacity-0" aria-hidden="true" />
    </button>
  );
}

function localChoiceDescription(
  t: (key: string) => string,
  choice: RepositoryPickerLocalChoice,
  state: RepositoryCloneSourceState | undefined,
  remoteOriginMode: boolean,
) {
  if (remoteOriginMode) {
    return (
      <span className="truncate text-[10px] text-muted-foreground">
        {cloneSourceStatusLabel(t, state)}
      </span>
    );
  }
  if (!choice.path) return null;
  return <span className="truncate text-[10px] text-muted-foreground">{choice.path}</span>;
}

function cloneSourceStatusLabel(
  t: (key: string) => string,
  state: RepositoryCloneSourceState | undefined,
): string {
  if (state?.status === "checking") return t("task:checkingRepositoryOrigin");
  if (state?.status === "ready") return t("task:cloneFromRemote");
  return t("task:noUsableRepositoryOrigin");
}

export function RemoteChoiceList({
  repositories,
  loading,
  error,
  onPick,
  onRetry,
}: {
  repositories: RemoteRepository[];
  loading: boolean;
  error?: Error;
  onPick: (repository: RemoteRepository) => void;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  if (loading && repositories.length === 0) {
    return (
      <div className="flex items-center gap-2 px-2 py-3 text-xs text-muted-foreground">
        <Spinner className="size-3" />
        <span>{t("task:loadingRepositories")}</span>
      </div>
    );
  }
  if (error) {
    return (
      <div
        className="flex items-center justify-between gap-2 px-2 py-3 text-xs text-destructive"
        role="alert"
      >
        <span className="min-w-0 break-words">
          {t("task:couldNotLoadRepositories", { message: error.message })}
        </span>
        <Button
          type="button"
          variant="outline"
          onClick={onRetry}
          className="min-h-11 shrink-0 cursor-pointer sm:min-h-8"
        >
          {t("task:retry")}
        </Button>
      </div>
    );
  }
  if (repositories.length === 0) {
    return <EmptyPickerMessage message={t("task:noRepositoriesFound")} />;
  }
  return (
    <div>
      {repositories.map((repository) => (
        <button
          type="button"
          key={`${repository.provider}:${repository.id}`}
          onClick={() => onPick(repository)}
          data-testid="task-repository-remote-option"
          className="flex min-h-11 w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-xs hover:bg-muted cursor-pointer sm:min-h-8"
        >
          <RemoteRepositoryProviderIcon provider={repository.provider} />
          <span className="truncate">{repository.fullName}</span>
        </button>
      ))}
    </div>
  );
}

function EmptyPickerMessage({ message }: { message: string }) {
  return <div className="px-2 py-3 text-xs text-muted-foreground">{message}</div>;
}
