"use client";

import { useMemo } from "react";
import { IconCheck, IconCircleDashed, IconX } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useOptimisticTaskMutation } from "@/hooks/use-optimistic-task-mutation";
import { formatRelativeTime } from "@/lib/utils";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import type { Task, TaskDecision } from "@/app/office/tasks/[id]/types";
import { MultiSelectPopover, type MultiSelectItem } from "./multi-select-popover";

type AgentItem = MultiSelectItem & { icon: string; name: string };

// AgentDecisionStatus is the per-chip badge state for an agent listed
// as a reviewer or approver. Computed by buildDecisionLookup below.
export type AgentDecisionStatus = "approved" | "changes_requested" | "pending";

// buildDecisionLookup maps agent id → most-recent decision for the
// given role, restricted to active rows. Used by ReviewersPicker and
// ApproversPicker to render a per-chip status icon.
export function buildDecisionLookup(
  decisions: TaskDecision[],
  role: TaskDecision["role"],
): Map<string, TaskDecision> {
  const out = new Map<string, TaskDecision>();
  for (const d of decisions) {
    if (d.role !== role) continue;
    const prev = out.get(d.deciderId);
    if (!prev || prev.createdAt < d.createdAt) {
      out.set(d.deciderId, d);
    }
  }
  return out;
}

function statusFromDecision(d: TaskDecision | undefined): AgentDecisionStatus {
  if (!d) return "pending";
  return d.decision === "approved" ? "approved" : "changes_requested";
}

type DecisionIconProps = {
  decision: TaskDecision | undefined;
};

const DECISION_ICON_CLASS: Record<AgentDecisionStatus, string> = {
  approved: "h-3 w-3 text-green-600",
  changes_requested: "h-3 w-3 text-red-600",
  pending: "h-3 w-3 text-muted-foreground",
};

const DECISION_ICON_COMPONENT: Record<AgentDecisionStatus, typeof IconCheck> = {
  approved: IconCheck,
  changes_requested: IconX,
  pending: IconCircleDashed,
};

function decisionTooltip(decision: TaskDecision | undefined): string {
  if (!decision) return "No decision yet";
  const verb = decision.decision === "approved" ? "Approved" : "Requested changes";
  const when = formatRelativeTime(decision.createdAt);
  const note = decision.comment ? ` — ${decision.comment}` : "";
  return `${verb} ${when}${note}`;
}

function DecisionIcon({ decision }: DecisionIconProps) {
  const status = statusFromDecision(decision);
  const Icon = DECISION_ICON_COMPONENT[status];
  const tip = decisionTooltip(decision);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span data-testid={`decision-icon-${status}`} aria-label={tip}>
          <Icon className={DECISION_ICON_CLASS[status]} />
        </span>
      </TooltipTrigger>
      <TooltipContent>{tip}</TooltipContent>
    </Tooltip>
  );
}

function buildAgentItems(agents: AgentProfile[]): AgentItem[] {
  return agents.map<AgentItem>((a) => ({
    id: a.id,
    name: a.name,
    icon: a.icon ?? "🤖",
    label: a.name,
    keywords: [a.name, a.role ?? ""],
  }));
}

type AgentsMultiPickerProps = {
  task: Task;
  selectedIds: string[];
  fieldKey: keyof Pick<Task, "reviewers" | "approvers">;
  addLabel: string;
  testId: string;
  apiAdd: (taskId: string, agentId: string) => Promise<unknown>;
  apiRemove: (taskId: string, agentId: string) => Promise<unknown>;
  // Optional map from agent id -> their most recent decision for this
  // picker's role. When provided, each chip renders a status icon next
  // to the agent name (✓ approved, ✕ changes requested, ◯ pending).
  decisionsByAgent?: Map<string, TaskDecision>;
};

export function AgentsMultiPicker({
  task,
  selectedIds,
  fieldKey,
  addLabel,
  testId,
  apiAdd,
  apiRemove,
  decisionsByAgent,
}: AgentsMultiPickerProps) {
  const agents = useAppStore((s) => s.office.agentProfiles);
  const mutate = useOptimisticTaskMutation();
  const items = useMemo(() => buildAgentItems(agents), [agents]);

  const handleAdd = async (id: string) => {
    if (selectedIds.includes(id)) return;
    const next = [...selectedIds, id];
    try {
      await mutate(task.id, { [fieldKey]: next } as Partial<Task>, () => apiAdd(task.id, id));
    } catch {
      /* hook toasts */
    }
  };

  const handleRemove = async (id: string) => {
    if (!selectedIds.includes(id)) return;
    const next = selectedIds.filter((a) => a !== id);
    try {
      await mutate(task.id, { [fieldKey]: next } as Partial<Task>, () => apiRemove(task.id, id));
    } catch {
      /* hook toasts */
    }
  };

  const renderChip = (item: AgentItem, remove: () => void) => (
    <span
      key={item.id}
      className="inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-xs"
    >
      <span className="text-sm leading-none">{item.icon}</span>
      <span className="truncate max-w-[110px]">{item.name}</span>
      {decisionsByAgent && <DecisionIcon decision={decisionsByAgent.get(item.id)} />}
      <span
        role="button"
        tabIndex={0}
        className="ml-0.5 cursor-pointer opacity-60 hover:opacity-100 inline-flex"
        onClick={(e) => {
          e.stopPropagation();
          remove();
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            e.stopPropagation();
            remove();
          }
        }}
        aria-label={`Remove ${item.name}`}
      >
        <IconX className="h-2.5 w-2.5" />
      </span>
    </span>
  );

  const renderItem = (item: AgentItem) => (
    <span className="flex items-center gap-2 min-w-0">
      <span className="text-base leading-none">{item.icon}</span>
      <span className="truncate">{item.name}</span>
    </span>
  );

  return (
    <MultiSelectPopover
      items={items}
      selectedIds={selectedIds}
      onAdd={handleAdd}
      onRemove={handleRemove}
      renderChip={renderChip}
      renderItem={renderItem}
      addLabel={addLabel}
      searchPlaceholder="Search agents..."
      emptyMessage="No agents found."
      testId={testId}
    />
  );
}
