import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { cn } from "@/lib/utils";
import type { ClassifyResult } from "@/lib/coordinator/attention";
import { linkToCoordinatorNeedsYou, linkToCoordinatorQueue } from "@/lib/coordinator/links";
import type { CoordinatorHeaderView } from "./coordinator-header";

export type CountStripProps = {
  classification: ClassifyResult;
  workspaceId: string;
  coordinatorId: string;
  view: CoordinatorHeaderView;
};

function CountLink({
  href,
  label,
  count,
  testId,
  current,
}: {
  href: string;
  label: string;
  count: number;
  testId: string;
  current: boolean;
}) {
  return (
    <Link
      href={href}
      aria-current={current ? "page" : undefined}
      className={cn(
        "flex flex-col items-center gap-0.5 px-3 py-1.5 first:rounded-l-md last:rounded-r-md",
        current ? "bg-primary/10 dark:bg-primary/20" : "hover:bg-accent",
      )}
      data-testid={testId}
    >
      <span className="text-lg font-semibold">{count}</span>
      <span
        className={cn("text-xs", current ? "text-primary font-medium" : "text-muted-foreground")}
      >
        {label}
      </span>
    </Link>
  );
}

/**
 * The sticky strip of counts above both screens, each linking to its list
 * (AC-COORDINATOR-NEEDS-YOU-003.1, .2). Counts come from the same
 * classification the lists render, so they always agree.
 *
 * The current screen is marked on every cell that leads to it, so the three
 * Queue cells are current together: the Queue screen shows all of its groups,
 * and `?group=` only opens one. The cells sit flush so that run reads as one
 * selected block rather than three independent selections.
 */
export function CountStrip({ classification, workspaceId, coordinatorId, view }: CountStripProps) {
  const { t } = useTranslation();
  const onQueue = view === "queue";
  return (
    <div className="flex items-center" data-testid="coordinator-count-strip">
      <div className="flex flex-wrap">
        <CountLink
          href={linkToCoordinatorNeedsYou(workspaceId, coordinatorId)}
          label={t("coordinator:countNeedsYou")}
          count={classification.needsYou.length}
          testId="count-needs-you"
          current={view === "needs-you"}
        />
        <CountLink
          href={linkToCoordinatorQueue(workspaceId, coordinatorId, "working")}
          label={t("coordinator:groupWorking")}
          count={classification.queue.working.length}
          testId="count-working"
          current={onQueue}
        />
        <CountLink
          href={linkToCoordinatorQueue(workspaceId, coordinatorId, "in_review")}
          label={t("coordinator:groupInReview")}
          count={classification.queue.in_review.length}
          testId="count-in-review"
          current={onQueue}
        />
        <CountLink
          href={linkToCoordinatorQueue(workspaceId, coordinatorId, "ready_to_merge")}
          label={t("coordinator:groupReadyToMerge")}
          count={classification.queue.ready_to_merge.length}
          testId="count-ready-to-merge"
          current={onQueue}
        />
      </div>
      {/* Beside the counts, not under them (AC-COORDINATOR-NEEDS-YOU-003.3):
          it is a caption on the strip, and reading as a paragraph of its own
          made it look like an annotation. Dropped where the row would wrap. */}
      <p className="text-muted-foreground ml-auto hidden pl-4 text-xs lg:block">
        {t("coordinator:positionsDerivedLine")}
      </p>
    </div>
  );
}
