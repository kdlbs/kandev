"use client";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { GroupKey } from "@/lib/state/slices/ui/sidebar-view-types";

const GROUP_OPTIONS: Array<{ key: GroupKey; label: string }> = [
  { key: "none", label: "None" },
  { key: "repository", label: "Repository" },
  { key: "workflow", label: "Workflow" },
  { key: "workflowStep", label: "Workflow step" },
  { key: "executorType", label: "Executor type" },
  { key: "state", label: "State" },
];

type Props = {
  value: GroupKey;
  onChange: (next: GroupKey) => void;
};

export function GroupPicker({ value, onChange }: Props) {
  return (
    <Select value={value} onValueChange={(v) => onChange(v as GroupKey)}>
      <SelectTrigger size="sm" className="h-7 w-full text-xs" data-testid="group-key-select">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {GROUP_OPTIONS.map((opt) => (
          <SelectItem key={opt.key} value={opt.key} className="text-xs">
            {opt.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
