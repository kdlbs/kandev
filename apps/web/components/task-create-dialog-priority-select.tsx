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
      <span className="text-xs text-muted-foreground">{t("kanban:priority")}</span>
      <Select
        value={value}
        onValueChange={(next) => onChange(next as TaskPriority)}
        disabled={disabled}
      >
        <SelectTrigger data-testid="task-create-priority-select" className="h-8 w-32" size="sm">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {KANBAN_PRIORITY_TOKENS.map((token) => (
            <SelectItem
              key={token}
              value={token}
              data-testid={`task-create-priority-option-${token}`}
            >
              {t(KANBAN_PRIORITY_LABEL_KEYS[token])}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
