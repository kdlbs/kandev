"use client";

import Link from "next/link";
import { useState } from "react";
import {
  IconShieldCheck,
  IconAlertTriangle,
  IconBug,
  IconEye,
  IconPlayerPause,
  IconRoute,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { useAppStore } from "@/components/state-provider";
import { dismissInboxItem, retryProvider } from "@/lib/api/domains/office-extended-api";
import type { InboxItem } from "@/lib/state/slices/office/types";
import { timeAgo } from "../components/shared/time-ago";

const ICON_MAP: Record<string, typeof IconShieldCheck> = {
  "shield-check": IconShieldCheck,
  "alert-triangle": IconAlertTriangle,
  bug: IconBug,
  eye: IconEye,
  "player-pause": IconPlayerPause,
  route: IconRoute,
};

const FALLBACK_TYPE_CONFIG: Record<string, { icon: typeof IconShieldCheck; label: string }> = {
  approval: { icon: IconShieldCheck, label: "Approval" },
  budget_alert: { icon: IconAlertTriangle, label: "Budget Alert" },
  agent_error: { icon: IconBug, label: "Agent Error" },
  agent_run_failed: { icon: IconBug, label: "Agent run failed" },
  agent_paused_after_failures: { icon: IconPlayerPause, label: "Agent auto-paused" },
  task_review: { icon: IconEye, label: "Task Review" },
  task_review_request: { icon: IconEye, label: "Review Request" },
  provider_degraded: { icon: IconRoute, label: "Provider degraded" },
};

// taskReviewHref returns the deep link for a `task_review_request`
// inbox item. The backend stamps the task id on `entity_id` for these
// rows; fall back to undefined so we render as plain text if the
// payload is malformed.
export function taskReviewHref(item: InboxItem): string | undefined {
  if (item.type !== "task_review_request") return undefined;
  if (!item.entity_id) return undefined;
  return `/office/tasks/${item.entity_id}`;
}

const statusBadgeClass: Record<string, string> = {
  pending: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-300",
  approved: "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300",
};
const defaultStatusBadge = "bg-muted text-muted-foreground";

type Props = {
  item: InboxItem;
  onApprove?: (id: string) => void;
  onReject?: (id: string) => void;
  /** Called after a successful Mark fixed so the parent can refetch. */
  onChanged?: () => void;
};

function useInboxTypeConfig(type: string) {
  const meta = useAppStore((s) => s.office.meta);
  const metaType = meta?.inboxItemTypes.find((t) => t.id === type);
  if (metaType) {
    return {
      icon: ICON_MAP[metaType.icon] ?? IconShieldCheck,
      label: metaType.label,
    };
  }
  return FALLBACK_TYPE_CONFIG[type] ?? FALLBACK_TYPE_CONFIG.approval;
}

function InboxRowBody({ item }: { item: InboxItem }) {
  const config = useInboxTypeConfig(item.type);
  const Icon = config.icon;
  return (
    <>
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
    </>
  );
}

export function InboxItemRow({ item, onApprove, onReject, onChanged }: Props) {
  const taskHref = taskReviewHref(item);
  if (taskHref) {
    return (
      <Link
        href={taskHref}
        className="flex items-center gap-3 px-4 py-2.5 hover:bg-accent/50 transition-colors cursor-pointer"
        data-testid={`inbox-item-${item.type}`}
      >
        <InboxRowBody item={item} />
      </Link>
    );
  }
  return (
    <div
      className="flex items-center gap-3 px-4 py-2.5 hover:bg-accent/50 transition-colors"
      data-testid={`inbox-item-${item.type}`}
    >
      <InboxRowBody item={item} />
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
      {(item.type === "agent_run_failed" || item.type === "agent_paused_after_failures") && (
        <MarkFixedButton item={item} onChanged={onChanged} />
      )}
      {item.type === "provider_degraded" && (
        <ProviderDegradedActions item={item} onChanged={onChanged} />
      )}
    </div>
  );
}

function ProviderDegradedActions({ item, onChanged }: { item: InboxItem; onChanged?: () => void }) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const [busy, setBusy] = useState(false);
  const action = (item.payload?.action as string | undefined) ?? "configure";
  const providerId = (item.payload?.provider_id as string | undefined) ?? "";

  const handleRetry = async () => {
    if (!workspaceId || !providerId || busy) return;
    setBusy(true);
    try {
      await retryProvider(workspaceId, providerId);
      onChanged?.();
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex gap-2 shrink-0">
      {action === "wait_for_capacity" ? (
        <Button size="sm" variant="outline" disabled className="cursor-pointer">
          Waiting…
        </Button>
      ) : (
        <Link
          href="/office/workspace/routing"
          className="text-xs underline-offset-4 hover:underline cursor-pointer"
        >
          {action === "reconnect" ? "Reconnect" : "Configure"}
        </Link>
      )}
      <Button
        size="sm"
        variant="outline"
        className="cursor-pointer"
        onClick={handleRetry}
        disabled={busy || !providerId}
      >
        {busy ? "Retrying…" : "Retry now"}
      </Button>
    </div>
  );
}

function MarkFixedButton({ item, onChanged }: { item: InboxItem; onChanged?: () => void }) {
  const [busy, setBusy] = useState(false);
  const handleClick = async () => {
    if (busy) return;
    if (item.type !== "agent_run_failed" && item.type !== "agent_paused_after_failures") {
      return;
    }
    setBusy(true);
    try {
      await dismissInboxItem(item.type, item.id);
      onChanged?.();
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="flex gap-2 shrink-0">
      <Button
        size="sm"
        className="cursor-pointer"
        onClick={handleClick}
        disabled={busy}
        data-testid={`inbox-mark-fixed-${item.type}`}
      >
        {busy ? "Marking…" : "Mark fixed"}
      </Button>
    </div>
  );
}
