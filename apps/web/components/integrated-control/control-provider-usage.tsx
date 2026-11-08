import { useTranslation } from "react-i18next";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { ControlModelActivity } from "./control-model-activity";
import { ControlModelBlocks } from "./control-model-blocks";
import { ControlProviderCircuits } from "./control-provider-circuits";
import { ControlCachedUsage } from "./control-cached-usage";
import Link from "@/components/routing/app-link";

export function ControlProviderUsage({ workspaceId }: { workspaceId?: string }) {
  const { t } = useTranslation();
  const limitsEnabled = useFeature("providerUsageLimits");
  return (
    <section className="space-y-4" data-testid="control-provider-usage">
      <Link
        className="inline-flex min-h-7 cursor-pointer items-center text-sm underline [@media(pointer:coarse)]:min-h-11"
        href="/settings/agents/dynamic"
      >
        {t("sidebar:controlRestore.selectionPolicies")}
      </Link>
      <ControlModelActivity workspaceId={workspaceId} />
      {limitsEnabled ? (
        <ControlCachedUsage workspaceId={workspaceId} />
      ) : (
        <p
          role="status"
          className="rounded-md border p-3 text-sm"
          data-testid="control-usage-unavailable"
        >
          {t("sidebar:controlInspection.usageDisabled")}
        </p>
      )}
      <ControlModelBlocks workspaceId={workspaceId} />
      <ControlProviderCircuits />
    </section>
  );
}
