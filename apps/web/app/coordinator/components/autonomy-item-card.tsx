import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from "@kandev/ui/card";
import Link from "@/components/routing/app-link";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { cn } from "@/lib/utils";
import type { NeedsYouAutonomyItem } from "@/lib/coordinator/attention";
import { heldFixText, heldReasonText } from "@/lib/coordinator/autonomy-text";
import { formatAge } from "@/lib/coordinator/format";
import { linkToCoordinatorAutonomySettings } from "@/lib/coordinator/links";

export type AutonomyItemCardProps = {
  item: NeedsYouAutonomyItem;
  workspaceId: string;
  coordinatorId: string;
  canManage: boolean;
};

/**
 * The held-autonomy Needs you item: the held reason as its title, how many
 * events wait, what clears it, and Open settings for a manager only.
 */
export function AutonomyItemCard({
  item,
  workspaceId,
  coordinatorId,
  canManage,
}: AutonomyItemCardProps) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const fix = heldFixText(item.reason, item.detail, item.conditions, t);
  return (
    <Card className="border-l-[3px] border-l-destructive" data-testid={`needs-you-item-${item.id}`}>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2 pr-2">
          <span data-testid="autonomy-item-title">
            {heldReasonText(item.reason, item.detail, t)}
          </span>
          <Badge variant="destructive">{t("coordinator:severityDecideNow")}</Badge>
          <span className="text-muted-foreground ml-auto font-normal">{formatAge(item.ageMs)}</span>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-2">
        <dl className="grid grid-cols-[minmax(82px,max-content)_minmax(0,1fr)] gap-x-2.5 gap-y-0.5">
          <dt className="text-muted-foreground">{t("coordinator:whyItIsHere")}</dt>
          <dd className="m-0 min-w-0 [overflow-wrap:anywhere]" data-testid="autonomy-item-why">
            {t("coordinator:autonomyWhy", { count: item.pendingWakes })}
          </dd>
          {fix && (
            <>
              <dt className="text-muted-foreground">{t("coordinator:whatClearsIt")}</dt>
              <dd className="m-0 min-w-0 [overflow-wrap:anywhere]" data-testid="autonomy-item-fix">
                {fix}
              </dd>
            </>
          )}
        </dl>
      </CardContent>
      {canManage && (
        <CardFooter>
          <Button
            asChild
            variant="outline"
            size="sm"
            className={cn(!isFinePointer && "min-h-11 min-w-11")}
          >
            <Link
              href={linkToCoordinatorAutonomySettings(workspaceId, coordinatorId)}
              data-testid="autonomy-item-open-settings"
            >
              {t("coordinator:autonomyOpenSettings")}
            </Link>
          </Button>
        </CardFooter>
      )}
    </Card>
  );
}
