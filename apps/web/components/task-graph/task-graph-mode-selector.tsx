import { useTranslation } from "react-i18next";
import { ToggleGroup, ToggleGroupItem } from "@kandev/ui/toggle-group";
import type { TaskGraphMode } from "@/lib/api/domains/task-graph-api";

const MODES: Array<[TaskGraphMode, string]> = [
  ["subtasks", "task:taskGraphModeSubtasks"],
  ["chain", "task:taskGraphModeChain"],
];

export function TaskGraphModeSelector({
  mode,
  available,
  disabled,
  onChange,
}: {
  mode: TaskGraphMode;
  available: TaskGraphMode[];
  disabled: boolean;
  onChange: (mode: TaskGraphMode) => void;
}) {
  const { t } = useTranslation();
  const subtasksUnavailable = !available.includes("subtasks");
  return (
    <div className="flex flex-wrap items-center gap-2">
      <ToggleGroup
        type="single"
        variant="outline"
        size="sm"
        value={mode}
        aria-label={t("task:taskGraphModeLabel")}
        onValueChange={(value) => {
          if (value) onChange(value as TaskGraphMode);
        }}
      >
        {MODES.map(([value, key]) => (
          <ToggleGroupItem
            key={value}
            value={value}
            disabled={disabled || !available.includes(value)}
            className="cursor-pointer px-3 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-24"
            data-testid={`task-graph-mode-${value}`}
          >
            {t(key)}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      {subtasksUnavailable && (
        <span className="text-[11px] text-muted-foreground">
          {t("task:taskGraphSubtasksUnavailable")}
        </span>
      )}
    </div>
  );
}
