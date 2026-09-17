import { useTranslation } from "react-i18next";
import { useFeature } from "@/hooks/domains/features/use-feature";
import Link from "@/components/routing/app-link";
import { coordinatorHref } from "@/lib/api/domains/orchestration-api";
export function WorkspaceOverviewOrchestration({ workspaceId }: { workspaceId: string }) {
  const enabled = useFeature("orchestration");
  const { t } = useTranslation();
  return enabled ? (
    <Link
      className="cursor-pointer block rounded-lg border p-4 space-y-2 hover:bg-muted/50"
      href={coordinatorHref(workspaceId)}
    >
      <h3 className="font-semibold">{t("orchestration:coordinator")}</h3>
      <p className="text-sm text-muted-foreground">{t("orchestration:coordinatorHint")}</p>
    </Link>
  ) : null;
}
