import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import type { ClassifyResult } from "@/lib/coordinator/attention";
import { linkToCoordinatorNeedsYou, linkToCoordinatorQueue } from "@/lib/coordinator/links";

export type CountStripProps = {
  classification: ClassifyResult;
  workspaceId: string;
  coordinatorId: string;
};

function CountLink({
  href,
  label,
  count,
  testId,
}: {
  href: string;
  label: string;
  count: number;
  testId: string;
}) {
  return (
    <Link
      href={href}
      className="flex flex-col items-center gap-0.5 rounded-md px-3 py-1.5 hover:bg-accent"
      data-testid={testId}
    >
      <span className="text-lg font-semibold">{count}</span>
      <span className="text-muted-foreground text-xs">{label}</span>
    </Link>
  );
}

/**
 * The sticky strip of counts above both screens, each linking to its list
 * (AC-COORDINATOR-NEEDS-YOU-003.1, .2). Counts come from the same
 * classification the lists render, so they always agree.
 */
export function CountStrip({ classification, workspaceId, coordinatorId }: CountStripProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1" data-testid="coordinator-count-strip">
      <div className="flex flex-wrap gap-2">
        <CountLink
          href={linkToCoordinatorNeedsYou(workspaceId, coordinatorId)}
          label={t("coordinator:countNeedsYou")}
          count={classification.needsYou.length}
          testId="count-needs-you"
        />
        <CountLink
          href={linkToCoordinatorQueue(workspaceId, coordinatorId, "working")}
          label={t("coordinator:groupWorking")}
          count={classification.queue.working.length}
          testId="count-working"
        />
        <CountLink
          href={linkToCoordinatorQueue(workspaceId, coordinatorId, "in_review")}
          label={t("coordinator:groupInReview")}
          count={classification.queue.in_review.length}
          testId="count-in-review"
        />
        <CountLink
          href={linkToCoordinatorQueue(workspaceId, coordinatorId, "ready_to_merge")}
          label={t("coordinator:groupReadyToMerge")}
          count={classification.queue.ready_to_merge.length}
          testId="count-ready-to-merge"
        />
      </div>
      <p className="text-muted-foreground text-xs">{t("coordinator:positionsDerivedLine")}</p>
    </div>
  );
}
