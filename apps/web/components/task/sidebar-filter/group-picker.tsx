"use client";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { GroupKey } from "@/lib/state/slices/ui/sidebar-view-types";
import { useTranslation } from "react-i18next";

// `labelKey` holds a catalog key, not copy: this table is module scope, so a
// resolved `t()` here would freeze at the boot locale. `key` is a persisted
// grouping value and stays in English.
const GROUP_OPTIONS: Array<{ key: GroupKey; labelKey: string }> = [
  { key: "none", labelKey: "task:groupNone" },
  { key: "repository", labelKey: "task:groupRepository" },
  { key: "workflow", labelKey: "task:groupWorkflow" },
  { key: "workflowStep", labelKey: "task:groupWorkflowStep" },
  { key: "executorType", labelKey: "task:groupExecutorType" },
  { key: "state", labelKey: "task:groupState" },
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
        {GROUP_OPTIONS.map((opt) => (
          <SelectItem key={opt.key} value={opt.key} className="text-xs">
            {t(opt.labelKey)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
