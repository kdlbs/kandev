import { useTranslation } from "react-i18next";
import { IconChevronRight } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import Link from "@/components/routing/app-link";
import { useRouter } from "@/lib/routing/client-router";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { cn } from "@/lib/utils";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import {
  linkToCoordinatorNeedsYou,
  linkToCoordinatorQueue,
  linkToCoordinatorSettings,
} from "@/lib/coordinator/links";

export type CoordinatorHeaderView = "needs-you" | "queue";

export type CoordinatorTitleSlotProps = {
  coordinator: Coordinator;
  coordinators: Coordinator[];
  workspaceId: string;
  view: CoordinatorHeaderView;
  title: string;
};

export type CoordinatorConfigureActionProps = {
  coordinator: Coordinator;
  workspaceId: string;
};

function hrefFor(workspaceId: string, coordinatorId: string, view: CoordinatorHeaderView): string {
  return view === "queue"
    ? linkToCoordinatorQueue(workspaceId, coordinatorId)
    : linkToCoordinatorNeedsYou(workspaceId, coordinatorId);
}

/**
 * The topbar's current-page crumb: the coordinator's name, or a selector of
 * the workspace's coordinators when there are several, then the screen's own
 * name (AC-COORDINATOR-NEEDS-YOU-006.5). It lives in `PageShell`'s `titleSlot`
 * because it carries an interactive control, which a plain `BreadcrumbPage`
 * must not announce as a disabled link.
 */
export function CoordinatorTitleSlot({
  coordinator,
  coordinators,
  workspaceId,
  view,
  title,
}: CoordinatorTitleSlotProps) {
  const { t } = useTranslation();
  const router = useRouter();

  return (
    <span className="flex min-w-0 items-center gap-1.5">
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
        <Link
          href={hrefFor(workspaceId, coordinator.id, view)}
          className="text-muted-foreground hover:text-foreground truncate text-sm transition-colors"
          data-testid="coordinator-crumb"
        >
          {coordinator.name}
        </Link>
      )}
      <IconChevronRight className="text-muted-foreground size-3.5 shrink-0" aria-hidden="true" />
      <span className="truncate text-sm font-medium">{title}</span>
    </span>
  );
}

/**
 * The topbar action to the coordinator's settings page, for managers
 * (AC-COORDINATOR-NEEDS-YOU-006.5).
 */
export function CoordinatorConfigureAction({
  coordinator,
  workspaceId,
}: CoordinatorConfigureActionProps) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  return (
    <Button
      asChild
      variant="outline"
      size="sm"
      className={cn(!isFinePointer && "min-h-11 min-w-11")}
    >
      <Link href={linkToCoordinatorSettings(workspaceId, coordinator.id)}>
        {t("coordinator:configure")}
      </Link>
    </Button>
  );
}
