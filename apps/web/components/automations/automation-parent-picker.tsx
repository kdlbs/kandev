"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { settingsControlClassName } from "@/components/settings/settings-control";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import { useAutomationParentCandidates } from "@/hooks/domains/settings/use-automation-parent-candidates";

const NO_PARENT = "__none__";

function taskOption(task: OfficeTask): ComboboxOption {
  return {
    value: task.id,
    label: `${task.identifier} ${task.title}`,
    keywords: [task.identifier, task.title],
    renderLabel: () => (
      <span className="flex min-w-0 items-center gap-2">
        <span className="shrink-0 font-mono text-xs text-muted-foreground">{task.identifier}</span>
        <span className="truncate">{task.title}</span>
      </span>
    ),
  };
}

function buildOptions(
  candidates: OfficeTask[],
  selectedId: string,
  noParentLabel: string,
): ComboboxOption[] {
  const noParent: ComboboxOption = {
    value: NO_PARENT,
    label: noParentLabel,
    keywords: [noParentLabel],
  };
  const options = candidates.map(taskOption);
  if (selectedId && !options.some((option) => option.value === selectedId)) {
    options.push({ value: selectedId, label: selectedId, keywords: [selectedId] });
  }
  return [noParent, ...options];
}

export function AutomationParentPicker({
  workspaceId,
  value,
  onChange,
  testId = "automation-parent-picker",
}: {
  workspaceId: string;
  value: string;
  onChange: (value: string) => void;
  testId?: string;
}) {
  const { t, i18n } = useTranslation();
  const [search, setSearch] = useState({ workspaceId, query: "" });
  const query = search.workspaceId === workspaceId ? search.query : "";
  const [selection, setSelection] = useState<{ workspaceId: string; task?: OfficeTask }>({
    workspaceId,
  });
  const { candidates, loading } = useAutomationParentCandidates(workspaceId, query);
  const selectedTask =
    selection.workspaceId === workspaceId && selection.task?.id === value
      ? selection.task
      : undefined;
  const noParentLabel = t("task:noParent");
  const options = useMemo(
    () =>
      buildOptions(
        selectedTask && !candidates.some((task) => task.id === selectedTask.id)
          ? [...candidates, selectedTask]
          : candidates,
        value,
        noParentLabel,
      ),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- i18n.language is the locale trigger
    [candidates, selectedTask, value, noParentLabel, i18n.language],
  );

  return (
    <Combobox
      options={options}
      value={value || NO_PARENT}
      onValueChange={(next) => {
        const id = next === NO_PARENT ? "" : next;
        setSelection({ workspaceId, task: candidates.find((task) => task.id === id) });
        onChange(id);
      }}
      onOpenChange={(open) => {
        const task = candidates.find((candidate) => candidate.id === value);
        if (open && task) setSelection({ workspaceId, task });
      }}
      searchValue={query}
      onSearchValueChange={(next) => setSearch({ workspaceId, query: next })}
      placeholder={noParentLabel}
      searchPlaceholder={t("task:searchTasks")}
      emptyMessage={t("task:noTasksFound")}
      loading={loading}
      triggerClassName={settingsControlClassName()}
      testId={testId}
    />
  );
}
