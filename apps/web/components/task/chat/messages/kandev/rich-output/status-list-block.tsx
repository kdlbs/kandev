import { useTranslation } from "react-i18next";
import {
  IconCircleCheck,
  IconCircleX,
  IconPlayerPlay,
  IconClock,
  IconPlayerSkipForward,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import type { StatusListProps } from "@/lib/rich-output/catalog";

const statuses = {
  passed: {
    Icon: IconCircleCheck,
    key: "task:richOutputStatusPassed",
    color: "text-green-700 dark:text-green-400",
    badge: "border-green-500/30 bg-green-500/10 dark:bg-green-500/10",
  },
  failed: {
    Icon: IconCircleX,
    key: "task:richOutputStatusFailed",
    color: "text-red-700 dark:text-red-400",
    badge: "border-red-500/30 bg-red-500/10 dark:bg-red-500/10",
  },
  running: {
    Icon: IconPlayerPlay,
    key: "task:richOutputStatusRunning",
    color: "text-blue-700 dark:text-blue-400",
    badge: "border-blue-500/30 bg-blue-500/10 dark:bg-blue-500/10",
  },
  pending: {
    Icon: IconClock,
    key: "task:richOutputStatusPending",
    color: "text-amber-700 dark:text-amber-400",
    badge: "border-amber-500/30 bg-amber-500/10 dark:bg-amber-500/10",
  },
  skipped: {
    Icon: IconPlayerSkipForward,
    key: "task:richOutputStatusSkipped",
    color: "text-muted-foreground",
    badge: "bg-muted/50 dark:bg-muted/50",
  },
} as const;

export function StatusList({ title, items }: StatusListProps) {
  const { t } = useTranslation();
  return (
    <section className="min-w-0 space-y-3" data-testid="rich-output-status-list">
      {title && <h4 className="text-xs font-medium">{title}</h4>}
      {items.length === 0 && (
        <p className="text-xs text-muted-foreground">{t("task:richOutputNoChecks")}</p>
      )}
      {items.length > 0 && (
        <ul className="space-y-3" aria-label={title || t("task:richOutputVerification")}>
          {items.map((item, index) => {
            const { Icon, key, color, badge } = statuses[item.status];
            return (
              <li key={index} className="flex min-w-0 gap-2 text-xs">
                <Icon className={`mt-0.5 size-4 shrink-0 ${color}`} aria-hidden />
                <div className="min-w-0 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="outline" className={`${badge} ${color}`} data-status-label>
                      {t(key)}
                    </Badge>
                    <span className="whitespace-pre-wrap wrap-anywhere font-medium">
                      {item.label}
                    </span>
                  </div>
                  {item.detail && (
                    <p className="whitespace-pre-wrap wrap-anywhere text-muted-foreground">
                      {item.detail}
                    </p>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
