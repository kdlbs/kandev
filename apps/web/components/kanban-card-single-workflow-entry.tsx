import { IconLogicBuffer } from "@tabler/icons-react";
import { t } from "@/lib/i18n";
import type { KanbanCardMenuEntry } from "./kanban-card-menu-items";

export function buildSingleTaskWorkflowEntry({
  currentWorkflowId,
  hasOtherWorkflows,
  disabled,
  onChangeWorkflow,
}: {
  currentWorkflowId?: string | null;
  hasOtherWorkflows: boolean;
  disabled?: boolean;
  onChangeWorkflow?: () => void;
}): KanbanCardMenuEntry | null {
  if (!currentWorkflowId || !hasOtherWorkflows || !onChangeWorkflow) return null;
  return {
    kind: "item",
    key: "change-workflow",
    testId: "task-context-change-workflow",
    icon: <IconLogicBuffer className="mr-2 h-4 w-4" />,
    label: t("task:changeWorkflow"),
    disabled,
    onSelect: onChangeWorkflow,
  };
}
