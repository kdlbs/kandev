import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { useTranslation } from "react-i18next";
import type { TasksListArchiveMode } from "@/lib/tasks/tasks-list-query";

export function TasksListArchiveModes({
  value,
  onChange,
}: {
  value: TasksListArchiveMode;
  onChange: (mode: TasksListArchiveMode) => void;
}) {
  const { t } = useTranslation();
  return (
    <ToggleGroup
      type="single"
      value={value}
      variant="outline"
      aria-label={t("tasks:archiveFilter")}
      data-testid="tasks-list-archive-modes"
      className="w-full shrink-0 md:w-auto"
      onValueChange={(next) => {
        if (next) onChange(next as TasksListArchiveMode);
      }}
    >
      {(["active", "archived", "all"] as const).map((mode) => (
        <ToggleGroupItem
          key={mode}
          value={mode}
          data-testid={`tasks-list-archive-${mode}`}
          className={controlSizingClassName(
            "standard",
            "min-w-0 flex-1 cursor-pointer px-3 text-sm md:flex-none",
          )}
        >
          {t(
            { active: "tasks:activeTasks", archived: "tasks:archived", all: "tasks:allTasks" }[
              mode
            ],
          )}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
