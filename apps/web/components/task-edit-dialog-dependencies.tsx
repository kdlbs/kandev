"use client";

import { IconInfoCircle } from "@tabler/icons-react";
import type { ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { TaskCreateDependencies } from "@/components/task-create-dialog-dependencies";
import type { TaskEditDialogDependenciesState } from "@/hooks/domains/task/use-task-edit-dialog-dependencies";
import { getTaskDependencyCycle } from "@/lib/api/domains/task-dependencies-api";

type TaskEditDialogDependenciesProps = {
  state: TaskEditDialogDependenciesState;
  disabled?: boolean;
};

function dependencySaveError(
  error: unknown,
  t: (key: string, options?: { cycle?: string }) => string,
): string {
  const cycle = getTaskDependencyCycle(error);
  if (cycle?.length) return t("task:dependencyCycleError", { cycle: cycle.join(" -> ") });
  return t("task:dependencyUpdateFailed");
}

export function TaskEditDialogDependencies({
  state,
  disabled = false,
}: TaskEditDialogDependenciesProps) {
  const { t } = useTranslation();
  const { loadError, saveError, candidateError } = state;
  let dependencyField: ReactNode;
  if (state.loading) {
    dependencyField = (
      <p
        className="px-1 text-xs text-muted-foreground"
        data-testid="task-edit-dependencies-loading"
      >
        {t("task:loadingDependencies")}
      </p>
    );
  } else if (loadError) {
    dependencyField = (
      <div className="flex min-h-12 items-center gap-2 px-1">
        <p className="min-w-0 flex-1 text-xs text-destructive" role="alert">
          {t("task:failedToLoadDependencies")}
        </p>
        <Button
          type="button"
          variant="ghost"
          className="h-11 min-h-11 shrink-0 cursor-pointer px-3 text-xs"
          onClick={state.retry}
          data-testid="task-edit-dependencies-retry"
        >
          {t("task:retry")}
        </Button>
      </div>
    );
  } else {
    dependencyField = (
      <TaskCreateDependencies
        value={state.draftIds}
        onChange={state.setDraftIds}
        disabled={disabled}
        candidates={state.candidates}
        selectedTitles={state.selectedTitles}
        candidatesLoading={state.candidatesLoading}
        searchValue={state.query}
        onSearchValueChange={state.setQuery}
        excludeTaskId={state.taskId ?? undefined}
      />
    );
  }

  return (
    <div className="min-w-0 space-y-1" data-testid="task-edit-dependencies">
      <div className="flex min-h-11 items-center gap-1 px-1 text-[11px] text-muted-foreground/70 md:min-h-6">
        <span>{t("task:dependsOn")}</span>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-11 min-h-11 w-11 min-w-11 cursor-pointer p-0 text-muted-foreground/70 hover:bg-transparent hover:text-muted-foreground md:h-6 md:min-h-6 md:w-6 md:min-w-6"
              aria-label={t("task:dependencyInfoLabel")}
              data-testid="task-edit-dependency-info"
            >
              <IconInfoCircle className="h-3.5 w-3.5" aria-hidden="true" />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="top" className="z-[60] max-w-xs">
            {t("task:dependencyInfo")}
          </TooltipContent>
        </Tooltip>
      </div>
      {dependencyField}
      {Boolean(candidateError) && !loadError && (
        <div className="flex min-h-11 items-center gap-2 px-1">
          <p className="min-w-0 flex-1 text-xs text-destructive" role="alert">
            {t("task:failedToLoadDependencyCandidates")}
          </p>
          <Button
            type="button"
            variant="ghost"
            className="h-11 min-h-11 shrink-0 cursor-pointer px-3 text-xs"
            onClick={state.retryCandidates}
            data-testid="task-edit-dependencies-candidates-retry"
          >
            {t("task:retry")}
          </Button>
        </div>
      )}
      {Boolean(saveError) && (
        <p
          className="px-1 text-xs text-destructive"
          role="alert"
          data-testid="task-edit-dependencies-error"
        >
          {dependencySaveError(saveError, t)}
        </p>
      )}
    </div>
  );
}
