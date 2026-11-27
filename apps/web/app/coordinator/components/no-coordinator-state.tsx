import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { linkToCoordinatorAdd } from "@/lib/coordinator/links";

export type NoCoordinatorStateProps = {
  workspaceId: string;
  canManage: boolean;
};

/**
 * Shown for `/workspaces/:id/coordinator` (and an unknown id) when the
 * workspace has no coordinator (AC-COORDINATOR-NEEDS-YOU-006.4,
 * AC-COORDINATOR-NEEDS-YOU-007.2). No count strip, no list.
 */
export function NoCoordinatorState({ workspaceId, canManage }: NoCoordinatorStateProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2 py-8 text-center" data-testid="no-coordinator-state">
      <p className="text-sm">{t("coordinator:noCoordinatorBody")}</p>
      {canManage && (
        <Button asChild variant="outline" size="sm">
          <Link href={linkToCoordinatorAdd(workspaceId)}>{t("coordinator:addACoordinator")}</Link>
        </Button>
      )}
    </div>
  );
}
