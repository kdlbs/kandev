import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import Link from "@/components/routing/app-link";
import { useRouter } from "@/lib/routing/client-router";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import {
  linkToCoordinatorNeedsYou,
  linkToCoordinatorQueue,
  linkToCoordinatorSettings,
} from "@/lib/coordinator/links";

export type CoordinatorHeaderView = "needs-you" | "queue";

export type CoordinatorHeaderProps = {
  coordinator: Coordinator;
  coordinators: Coordinator[];
  workspaceId: string;
  view: CoordinatorHeaderView;
  canManage: boolean;
};

function hrefFor(workspaceId: string, coordinatorId: string, view: CoordinatorHeaderView): string {
  return view === "queue"
    ? linkToCoordinatorQueue(workspaceId, coordinatorId)
    : linkToCoordinatorNeedsYou(workspaceId, coordinatorId);
}

/**
 * The screens' header: the coordinator's name, or a selector when the
 * workspace has several, and Configure for managers
 * (AC-COORDINATOR-NEEDS-YOU-006.5).
 */
export function CoordinatorHeader({
  coordinator,
  coordinators,
  workspaceId,
  view,
  canManage,
}: CoordinatorHeaderProps) {
  const { t } = useTranslation();
  const router = useRouter();

  return (
    <div className="flex items-center justify-between gap-2">
      {coordinators.length > 1 ? (
        <Select
          value={coordinator.id}
          onValueChange={(value) => router.push(hrefFor(workspaceId, value, view))}
        >
          <SelectTrigger
            aria-label={t("coordinator:selectCoordinator")}
            data-testid="coordinator-selector"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {coordinators.map((c) => (
              <SelectItem key={c.id} value={c.id}>
                {c.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : (
        <h2 className="text-lg font-semibold">{coordinator.name}</h2>
      )}
      {canManage && (
        <Button asChild variant="outline" size="sm">
          <Link href={linkToCoordinatorSettings(workspaceId, coordinator.id)}>
            {t("coordinator:configure")}
          </Link>
        </Button>
      )}
    </div>
  );
}
