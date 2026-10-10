import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { linkToCoordinatorSettingsList } from "@/lib/coordinator/links";

export type UnknownCoordinatorStateProps = {
  workspaceId: string;
};

/**
 * A coordinator id that is not in this workspace, which has at least one
 * coordinator: no count strip, no list, no Add a coordinator
 * (AC-COORDINATOR-NEEDS-YOU-006.4).
 */
export function UnknownCoordinatorState({ workspaceId }: UnknownCoordinatorStateProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2 py-8 text-center" data-testid="unknown-coordinator-state">
      <p className="text-sm">{t("coordinator:unknownCoordinatorTitle")}</p>
      <Button asChild variant="outline" size="sm">
        <Link href={linkToCoordinatorSettingsList(workspaceId)}>
          {t("coordinator:seeCoordinators")}
        </Link>
      </Button>
    </div>
  );
}
