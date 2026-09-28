import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { linkToCoordinatorQueue } from "@/lib/coordinator/links";

export type EmptyNeedsYouStateProps = {
  workingCount: number;
  workspaceId: string;
  coordinatorId: string;
};

/**
 * Needs you's empty state: an empty list reads as success, with the Working
 * count and a link to see it (AC-COORDINATOR-NEEDS-YOU-007.1).
 */
export function EmptyNeedsYouState({
  workingCount,
  workspaceId,
  coordinatorId,
}: EmptyNeedsYouStateProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2 py-8 text-center" data-testid="empty-needs-you-state">
      <p className="text-sm font-medium">{t("coordinator:emptyNeedsYouTitle")}</p>
      <p className="text-muted-foreground text-sm">
        {t("coordinator:emptyNeedsYouWorkingCount", { count: workingCount })}
      </p>
      <Button asChild variant="outline" size="sm">
        <Link href={linkToCoordinatorQueue(workspaceId, coordinatorId, "working")}>
          {t("coordinator:seeWhatIsRunning")}
        </Link>
      </Button>
    </div>
  );
}
