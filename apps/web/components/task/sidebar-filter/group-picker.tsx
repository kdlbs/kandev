"use client";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { GroupKey } from "@/lib/state/slices/ui/sidebar-view-types";
import { useTranslation } from "react-i18next";

// `labelKey` holds a catalog key, not copy: this table is module scope, so a
// resolved `t()` here would freeze at the boot locale. `key` is a persisted
// grouping value and stays in English.
const GROUP_OPTIONS: Array<{ key: GroupKey; labelKey: string; descriptionKey: string }> = [
  {
    key: "none",
    labelKey: "task:groupNone",
    descriptionKey: "task:groupNoneDescription",
  },
  {
    key: "repository",
    labelKey: "task:groupRepository",
    descriptionKey: "task:groupRepositoryDescription",
  },
  {
    key: "workflow",
    labelKey: "task:groupWorkflow",
    descriptionKey: "task:groupWorkflowDescription",
  },
  {
    key: "workflowStep",
    labelKey: "task:groupWorkflowStep",
    descriptionKey: "task:groupWorkflowStepDescription",
  },
  {
    key: "executorType",
    labelKey: "task:groupExecutorType",
    descriptionKey: "task:groupExecutorTypeDescription",
  },
  {
    key: "state",
    labelKey: "task:groupState",
    descriptionKey: "task:groupStateDescription",
  },
];

type Props = {
  value: GroupKey;
  onChange: (next: GroupKey) => void;
};

export function GroupPicker({ value, onChange }: Props) {
  const { t } = useTranslation();
  return (
    <Select value={value} onValueChange={(v) => onChange(v as GroupKey)}>
      <SelectTrigger size="sm" className="h-7 w-full text-xs" data-testid="group-key-select">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {GROUP_OPTIONS.map((opt) => {
          const label = t(opt.labelKey);
          return (
            <SelectItem
              key={opt.key}
              value={opt.key}
              className="text-xs"
              aria-label={label}
              description={t(opt.descriptionKey)}
            >
              {label}
            </SelectItem>
          );
        })}
      </SelectContent>
    </Select>
  );
}
