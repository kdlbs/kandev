"use client";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { BuiltinGroupKey, GroupKey } from "@/lib/state/slices/ui/sidebar-view-types";
import { usePluginRegistry } from "@/lib/plugins/registry";
import { encodeTaskListFacetOption, isTaskListFacetOption } from "@/lib/tasks/tasks-list-options";
import { useTranslation } from "react-i18next";

// `labelKey` holds a catalog key, not copy: this table is module scope, so a
// resolved `t()` here would freeze at the boot locale. `key` is a persisted
// grouping value and stays in English.
const GROUP_OPTIONS: Array<{ key: BuiltinGroupKey; labelKey: string; descriptionKey: string }> = [
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

export function useSidebarGroupLabel(value: GroupKey): string {
  const { t } = useTranslation();
  const registry = usePluginRegistry();
  if (isTaskListFacetOption(value)) {
    return (
      registry.getTaskListFacets().find((facet) => encodeTaskListFacetOption(facet) === value)
        ?.label ?? t("task:groupNone")
    );
  }
  return t(GROUP_OPTIONS.find((option) => option.key === value)?.labelKey ?? "task:groupNone");
}

type Props = {
  value: GroupKey;
  onChange: (next: GroupKey) => void;
};

export function GroupPicker({ value, onChange }: Props) {
  const { t } = useTranslation();
  const registry = usePluginRegistry();
  const facets = registry.getTaskListFacets();
  const available =
    GROUP_OPTIONS.some((option) => option.key === value) ||
    facets.some((facet) => encodeTaskListFacetOption(facet) === value);
  return (
    <Select value={available ? value : "none"} onValueChange={(v) => onChange(v as GroupKey)}>
      <SelectTrigger className="w-full text-xs" data-testid="group-key-select">
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
        {facets.map((facet) => (
          <SelectItem
            key={encodeTaskListFacetOption(facet)}
            value={encodeTaskListFacetOption(facet)}
            className="text-xs"
            aria-label={facet.label}
          >
            {facet.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
