import { useTranslation } from "react-i18next";
import { useFeature } from "@/hooks/domains/features/use-feature";
import Link from "@/components/routing/app-link";
import { orchestratorsHref } from "@/lib/api/domains/orchestration-api";
export function WorkspaceOverviewOrchestration({ workspaceId }: { workspaceId: string }) {
  const enabled = useFeature("orchestration");
  const { t } = useTranslation();
  return enabled ? (
    <Link
      className="block rounded-lg border p-4 space-y-2 hover:bg-muted/50"
      href={orchestratorsHref(workspaceId)}
    >
      <h3 className="font-semibold">{t("orchestration:orchestration")}</h3>
      <p className="text-sm text-muted-foreground">{t("orchestration:orchestratorsHint")}</p>
    </Link>
  ) : null;
}
