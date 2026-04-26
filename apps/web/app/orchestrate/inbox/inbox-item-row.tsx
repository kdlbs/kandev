"use client";

import {
  IconShieldCheck,
  IconAlertTriangle,
  IconBug,
  IconEye,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import type { InboxItem } from "@/lib/state/slices/orchestrate/types";

function timeAgo(dateStr: string): string {
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  const diffMs = now - then;
  const diffMin = Math.floor(diffMs / 60_000);
  if (diffMin < 1) return "just now";
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return `${diffHr}h ago`;
  const diffDay = Math.floor(diffHr / 24);
  return `${diffDay}d ago`;
}

const typeConfig: Record<string, { icon: typeof IconShieldCheck; label: string }> = {
  approval: { icon: IconShieldCheck, label: "Approval" },
  budget_alert: { icon: IconAlertTriangle, label: "Budget Alert" },
  agent_error: { icon: IconBug, label: "Agent Error" },
  task_review: { icon: IconEye, label: "Task Review" },
};

type Props = {
  item: InboxItem;
  onApprove?: (id: string) => void;
  onReject?: (id: string) => void;
};

export function InboxItemRow({ item, onApprove, onReject }: Props) {
  const config = typeConfig[item.type] ?? typeConfig.approval;
  const Icon = config.icon;

  return (
    <div className="flex items-center gap-3 px-4 py-3 hover:bg-accent/50 transition-colors">
      <div className="h-8 w-8 rounded-full bg-muted flex items-center justify-center shrink-0">
        <Icon className="h-4 w-4 text-muted-foreground" />
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium truncate">{item.title}</p>
        {item.description && (
          <p className="text-xs text-muted-foreground truncate">{item.description}</p>
        )}
        <p className="text-xs text-muted-foreground">{timeAgo(item.createdAt)}</p>
      </div>
      <Badge
        className={
          item.status === "pending"
            ? "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-300"
            : item.status === "approved"
              ? "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300"
              : "bg-muted text-muted-foreground"
        }
      >
        {item.status}
      </Badge>
      {item.type === "approval" && item.status === "pending" && (
        <div className="flex gap-2 shrink-0">
          <Button
            size="sm"
            className="bg-green-700 text-white hover:bg-green-800 cursor-pointer"
            onClick={() => onApprove?.(item.id)}
          >
            Approve
          </Button>
          <Button
            size="sm"
            variant="destructive"
            className="cursor-pointer"
            onClick={() => onReject?.(item.id)}
          >
            Reject
          </Button>
        </div>
      )}
    </div>
  );
}
