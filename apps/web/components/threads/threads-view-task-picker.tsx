"use client";

import { useId, useMemo, useState } from "react";
import { IconArrowLeft, IconCheck } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Input } from "@kandev/ui/input";
import { useTranslation } from "react-i18next";
import { PRTaskIcon } from "@/components/github/pr-task-icon";
import {
  getTaskStateIconLabelKey,
  TaskStateIcon,
  type TaskStateIconProps,
} from "@/components/task/task-state-icon";
import type { ThreadCandidate } from "@/lib/threads/thread-view-query";
import {
  MAX_THREAD_VIEW_TASK_IDS,
  updateThreadTaskSelection,
  updateVisibleThreadTaskSelection,
} from "./threads-view-editor-utils";

type Props = {
  candidates: ThreadCandidate[];
  selectedTaskIds: string[];
  onChange: (taskIds: string[]) => void;
  onBack: () => void;
};

function selectAllValue(
  allVisibleSelected: boolean,
  someVisibleSelected: boolean,
): boolean | "indeterminate" {
  if (allVisibleSelected) return true;
  if (someVisibleSelected) return "indeterminate";
  return false;
}

function candidateStateIconProps(candidate: ThreadCandidate): TaskStateIconProps {
  const pendingAction = candidate.taskPendingAction ?? candidate.pendingAction;
  return {
    state: candidate.taskState ?? undefined,
    sessionState: candidate.sessionState,
    foregroundActivity: candidate.foregroundActivity,
    hasPendingClarification: pendingAction === "clarification",
    hasPendingPermission: pendingAction === "permission",
    isOnLastWorkflowStep: candidate.isOnLastWorkflowStep,
    interrupted: candidate.interrupted,
  };
}

function ThreadTaskPickerRow({
  candidate,
  checked,
  onCheckedChange,
}: {
  candidate: ThreadCandidate;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  const { t } = useTranslation();
  const checkboxId = useId();
  const iconProps = candidateStateIconProps(candidate);

  return (
    <div
      className="flex min-h-11 items-center gap-2 rounded-md px-2 text-sm hover:bg-muted/60"
      data-testid="threads-task-picker-row"
    >
      <Checkbox
        id={checkboxId}
        checked={checked}
        onCheckedChange={(value) => onCheckedChange(value === true)}
        aria-label={candidate.title}
        data-testid="threads-task-picker-checkbox"
      />
      <label
        htmlFor={checkboxId}
        className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 py-2"
      >
        <TaskStateIcon {...iconProps} accessibleLabel={t(getTaskStateIconLabelKey(iconProps))} />
        <span className="min-w-0 flex-1 truncate">{candidate.title}</span>
        {candidate.stepTitle && (
          <Badge
            variant="secondary"
            className="shrink-0 text-[0.6rem]"
            data-testid="threads-task-picker-step"
          >
            {candidate.stepTitle}
          </Badge>
        )}
      </label>
      <PRTaskIcon taskId={candidate.taskId} prInfo={candidate.prInfo} />
      {checked && <IconCheck className="h-4 w-4 shrink-0 text-primary" />}
    </div>
  );
}

// eslint-disable-next-line max-lines-per-function -- Keeps search, bulk selection, and task-row updates in one picker boundary.
export function ThreadsViewTaskPicker({ candidates, selectedTaskIds, onChange, onBack }: Props) {
  const { t } = useTranslation();
  const [search, setSearch] = useState("");
  const selected = useMemo(() => new Set(selectedTaskIds), [selectedTaskIds]);
  const visibleCandidates = useMemo(() => {
    const needle = search.trim().toLocaleLowerCase();
    if (!needle) return candidates;
    return candidates.filter((candidate) => candidate.title.toLocaleLowerCase().includes(needle));
  }, [candidates, search]);
  const visibleIds = visibleCandidates.map((candidate) => candidate.taskId);
  const selectedVisibleCount = visibleIds.filter((taskId) => selected.has(taskId)).length;
  const allVisibleSelected = visibleIds.length > 0 && selectedVisibleCount === visibleIds.length;
  const someVisibleSelected = selectedVisibleCount > 0 && !allVisibleSelected;

  function setSelected(taskId: string, checked: boolean) {
    onChange(updateThreadTaskSelection(selectedTaskIds, taskId, checked));
  }

  function setAllVisible(checked: boolean) {
    onChange(updateVisibleThreadTaskSelection(selectedTaskIds, visibleIds, checked));
  }

  return (
    <div className="flex min-h-0 flex-col" data-testid="threads-task-picker">
      <div className="flex shrink-0 items-center gap-2 border-b p-2">
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-9 w-9 shrink-0 cursor-pointer"
          onClick={onBack}
          aria-label={t("threads:backToViewEditor")}
          data-testid="threads-task-picker-back"
        >
          <IconArrowLeft className="h-4 w-4" />
        </Button>
        <div className="min-w-0">
          <p className="text-sm font-medium">{t("threads:chooseTasks")}</p>
          <p className="text-xs text-muted-foreground">
            {t("threads:selectedTaskCount", { count: selected.size })}
          </p>
        </div>
      </div>
      <div className="shrink-0 space-y-2 border-b p-2">
        <Input
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder={t("threads:searchTasks")}
          aria-label={t("threads:searchTasks")}
          className="min-h-11"
          data-testid="threads-task-picker-search"
        />
        <div className="flex items-center justify-between gap-2 text-xs">
          <label className="flex min-h-11 cursor-pointer items-center gap-2">
            <Checkbox
              checked={selectAllValue(allVisibleSelected, someVisibleSelected)}
              onCheckedChange={(checked) => setAllVisible(checked === true)}
              aria-label={t("threads:selectAll")}
              data-testid="threads-task-picker-select-all"
            />
            <span>{t("threads:selectAll")}</span>
          </label>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="min-h-11 cursor-pointer px-2 text-xs"
            onClick={() => onChange([])}
            data-testid="threads-task-picker-clear-all"
          >
            {t("threads:clearAll")}
          </Button>
        </div>
        {selected.size >= MAX_THREAD_VIEW_TASK_IDS && (
          <p className="text-xs text-muted-foreground" role="status">
            {t("threads:taskSelectionLimitReached", { count: MAX_THREAD_VIEW_TASK_IDS })}
          </p>
        )}
      </div>
      <div className="p-1" data-testid="threads-task-picker-list">
        {visibleCandidates.length === 0 ? (
          <p className="p-3 text-center text-xs text-muted-foreground">
            {t("threads:noTasksToChoose")}
          </p>
        ) : (
          visibleCandidates.map((candidate) => (
            <ThreadTaskPickerRow
              key={candidate.taskId}
              candidate={candidate}
              checked={selected.has(candidate.taskId)}
              onCheckedChange={(checked) => setSelected(candidate.taskId, checked)}
            />
          ))
        )}
      </div>
    </div>
  );
}
