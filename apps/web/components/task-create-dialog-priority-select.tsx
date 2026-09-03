"use client";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useTranslation } from "react-i18next";
import { KANBAN_PRIORITY_LABEL_KEYS, KANBAN_PRIORITY_TOKENS } from "@/lib/kanban/task-priority";
import type { TaskPriority } from "@/lib/types/http";

export function TaskCreatePrioritySelect({
  value,
  onChange,
  disabled,
}: {
  value: TaskPriority;
  onChange: (value: TaskPriority) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2">
      <label htmlFor="task-create-priority-select" className="text-xs text-muted-foreground">
        {t("kanban:priority")}
      </label>
      <Select
        value={value}
        onValueChange={(next) => onChange(next as TaskPriority)}
        disabled={disabled}
      >
        <SelectTrigger
          id="task-create-priority-select"
          data-testid="task-create-priority-select"
          aria-label={t("kanban:priority")}
          className="h-11 min-h-11 w-32 sm:h-8 sm:min-h-0"
          size="sm"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {KANBAN_PRIORITY_TOKENS.map((token) => (
            <SelectItem
              key={token}
              value={token}
              data-testid={`task-create-priority-option-${token}`}
              className="min-h-11 sm:min-h-7"
            >
              {t(KANBAN_PRIORITY_LABEL_KEYS[token])}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
