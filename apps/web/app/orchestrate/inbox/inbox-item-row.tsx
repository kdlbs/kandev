"use client";

import { IconShieldCheck, IconAlertTriangle, IconBug, IconEye } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { useAppStore } from "@/components/state-provider";
import type { InboxItem } from "@/lib/state/slices/orchestrate/types";
import { timeAgo } from "../components/shared/time-ago";

const ICON_MAP: Record<string, typeof IconShieldCheck> = {
  "shield-check": IconShieldCheck,
  "alert-triangle": IconAlertTriangle,
  bug: IconBug,
  eye: IconEye,
};

const FALLBACK_TYPE_CONFIG: Record<string, { icon: typeof IconShieldCheck; label: string }> = {
  approval: { icon: IconShieldCheck, label: "Approval" },
  budget_alert: { icon: IconAlertTriangle, label: "Budget Alert" },
  agent_error: { icon: IconBug, label: "Agent Error" },
  task_review: { icon: IconEye, label: "Task Review" },
};

const statusBadgeClass: Record<string, string> = {
  pending: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-300",
  approved: "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300",
};
const defaultStatusBadge = "bg-muted text-muted-foreground";

type Props = {
  item: InboxItem;
  onApprove?: (id: string) => void;
  onReject?: (id: string) => void;
};

function useInboxTypeConfig(type: string) {
  const meta = useAppStore((s) => s.orchestrate.meta);
  const metaType = meta?.inboxItemTypes.find((t) => t.id === type);
  if (metaType) {
    return {
      icon: ICON_MAP[metaType.icon] ?? IconShieldCheck,
      label: metaType.label,
    };
  }
  return FALLBACK_TYPE_CONFIG[type] ?? FALLBACK_TYPE_CONFIG.approval;
}

export function InboxItemRow({ item, onApprove, onReject }: Props) {
  const config = useInboxTypeConfig(item.type);
  const Icon = config.icon;

  return (
    <div className="flex items-center gap-3 px-4 py-2.5 hover:bg-accent/50 transition-colors">
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
      <Badge className={statusBadgeClass[item.status] ?? defaultStatusBadge}>{item.status}</Badge>
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
