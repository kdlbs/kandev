"use client";

import Link from "next/link";
import { Badge } from "@kandev/ui/badge";
import { StatusIcon } from "@/app/office/tasks/[id]/status-icon";
import { topoSort } from "./workflow-sort";
import type { Task } from "@/app/office/tasks/[id]/types";
import type { TreeHold } from "@/lib/api/domains/tree-api";

type SubtaskStepperProps = {
  items: Task["children"];
  activeHold: TreeHold | null;
};

export function SubtaskStepper({ items, activeHold }: SubtaskStepperProps) {
  if (items.length === 0) return null;

  const sorted = topoSort(items);
  const completedIds = new Set(
    items
      .filter((i) => {
        const s = i.status?.toLowerCase();
        return s === "done" || s === "completed" || s === "cancelled";
      })
      .map((i) => i.id),
  );
  const holdLabel = activeHold?.mode === "pause" ? "Paused" : "Cancelled (tree)";

  return (
    <div className="mt-8" data-testid="subtask-stepper">
      <h2 className="text-sm font-semibold mb-4">Sub-tasks</h2>
      <div className="relative">
        <div className="absolute left-4 top-0 bottom-0 w-px bg-border" />
        <div className="space-y-1 pl-10">
          {sorted.map((child, idx) => {
            const pendingBlockers = (child.blockedBy ?? []).filter((b) => !completedIds.has(b));
            const isBlocked = pendingBlockers.length > 0;

            let dotClass = "bg-background border-border";
            if (completedIds.has(child.id)) {
              dotClass = "bg-primary border-primary text-primary-foreground";
            } else if (isBlocked) {
              dotClass = "bg-muted border-muted-foreground/30";
            }

            return (
              <div key={child.id} className="relative">
                <div
                  className={`absolute -left-6 top-1/2 -translate-y-1/2 h-4 w-4 rounded-full border-2 flex items-center justify-center text-[10px] font-bold ${dotClass}`}
                >
                  {completedIds.has(child.id) ? "✓" : idx + 1}
                </div>
                <Link
                  href={`/office/tasks/${child.id}`}
                  className={`flex items-center gap-2 px-4 py-2.5 text-sm rounded-md hover:bg-accent/50 transition-colors${isBlocked ? " opacity-60" : ""}`}
                >
                  <StatusIcon status={child.status} className="h-3.5 w-3.5 shrink-0" />
                  <span className="text-xs text-muted-foreground font-mono shrink-0">
                    {child.identifier}
                  </span>
                  <span className="flex-1 truncate">{child.title}</span>
                  {isBlocked && (
                    <Badge variant="outline" className="text-xs shrink-0">
                      Blocked
                    </Badge>
                  )}
                  {activeHold && !isBlocked && (
                    <Badge variant="outline" className="text-xs shrink-0">
                      {holdLabel}
                    </Badge>
                  )}
                </Link>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
