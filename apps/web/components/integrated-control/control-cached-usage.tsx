import { useTranslation } from "react-i18next";
import { useControlCachedUsage } from "@/hooks/domains/coordinator/use-control-cached-usage";
import { groupExecutionUsage } from "@/lib/control-ui/usage";
import { useIntegratedControl } from "./integrated-control-provider";
import { ControlProviderAccount } from "./control-provider-account";
import { ControlReadState } from "./control-read-state";
import { ControlUnattributedUsage } from "./control-unattributed-usage";

export function ControlCachedUsage({ workspaceId }: { workspaceId?: string }) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const workspaces = (control?.data?.workspaces ?? []).filter(
    (workspace) => !workspaceId || workspace.id === workspaceId,
  );
  const result = useControlCachedUsage(
    workspaces.map((workspace) => workspace.id),
    control?.data,
  );
  const groups = groupExecutionUsage(result.data?.flatMap((profile) => profile.entries) ?? []);
  const failures = [
    ...new Map(
      result.data
        ?.filter((profile) => profile.status !== "ready")
        .map((profile) => [
          JSON.stringify([profile.status, "receivedAt" in profile ? profile.receivedAt : null]),
          profile,
        ]),
    ).values(),
  ];
  return (
    <section
      className="space-y-3"
      data-testid="control-cached-usage"
      aria-busy={result.status === "loading" || Boolean(result.refreshing)}
    >
      <ControlReadState {...result} />
      {failures.map((profile) => (
        <ControlReadState key={profile.id} {...profile} />
      ))}
      {result.status === "ready" && groups.length === 0 && (
        <p
          role="status"
          className="rounded-md border p-3 text-sm"
          data-testid="control-usage-unavailable"
        >
          {t("sidebar:controlInspection.usageUnknown")}
        </p>
      )}
      {groups
        .filter((group) => group.sample)
        .map((group) => (
          <ControlProviderAccount key={group.key} group={group} />
        ))}
      <ControlUnattributedUsage
        entries={groups.filter((group) => !group.sample).flatMap((group) => group.entries)}
      />
    </section>
  );
}
