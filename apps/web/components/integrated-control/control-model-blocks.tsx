import { useTranslation } from "react-i18next";
import { useControlProviderHealth } from "@/hooks/domains/coordinator/use-control-provider-health";
import type { ProviderHealth } from "@/lib/state/slices/office/types";
import { parseTurnTimestamp } from "@/lib/state/slices/session/turn-actions";
import { formatDateTime } from "@/lib/i18n/formats";
import { useIntegratedControl } from "./integrated-control-provider";
import { ControlReadState } from "./control-read-state";
import { SectionCard } from "./overview-section-card";

const STATE_KEYS = {
  healthy: "office:healthy",
  short_retry: "office:shortRetry",
  degraded: "office:degraded",
  user_action_required: "office:needsAction",
};

export function ControlModelBlocks({ workspaceId }: { workspaceId?: string }) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const workspaces = (control?.data?.workspaces ?? []).filter(
    (workspace) => !workspaceId || workspace.id === workspaceId,
  );
  const result = useControlProviderHealth(
    workspaces.map((workspace) => workspace.id),
    control?.data,
  );
  const nameOf = (id: string) => workspaces.find((item) => item.id === id)?.name ?? id;
  const isEmpty = (row: { status: string; health: unknown[] }) =>
    row.status === "ready" && row.health.length === 0;
  const empty = result.data?.filter(isEmpty) ?? [];
  return (
    <SectionCard title={t("office:providerHealth")}>
      <ControlReadState {...result} />
      {empty.length > 0 && (
        <p className="border-t p-3 text-sm text-muted-foreground">
          {t("sidebar:controlRestore.noProviderHealth")}{" "}
          <span className="text-xs">
            ({empty.map((workspace) => nameOf(workspace.id)).join(", ")})
          </span>
        </p>
      )}
      {result.data
        ?.filter((workspace) => !isEmpty(workspace))
        .map((workspace) => (
          <section
            key={workspace.id}
            className="space-y-2 border-t p-3 text-sm"
            data-testid="control-provider-health"
          >
            <h3 className="font-medium">{nameOf(workspace.id)}</h3>
            <ControlReadState {...workspace} />
            <div className="grid gap-2 sm:grid-cols-2">
              {workspace.health.map((row) => (
                <ProviderHealthRow
                  key={row.provider_id + ":" + row.scope + ":" + row.scope_value}
                  row={row}
                />
              ))}
            </div>
          </section>
        ))}
    </SectionCard>
  );
}

function ProviderHealthRow({ row }: { row: ProviderHealth }) {
  const { t } = useTranslation();
  const retry = healthTime(row.retry_at);
  return (
    <div
      className="space-y-1 break-words rounded-md border p-3"
      data-testid="control-provider-health-row"
    >
      <h4 className="font-medium">
        {row.provider_id} {row.scope_value}
      </h4>
      <p>{t(STATE_KEYS[row.state])}</p>
      {row.error_code && <p>{row.error_code}</p>}
      <p>
        {t("sidebar:controlRestore.healthScope")}: {row.scope}
      </p>
      <p>
        {t("sidebar:controlRestore.recordedRetryAt")}: {retry ?? t("common:unknown")}
      </p>
      {retry && Date.parse(row.retry_at!) <= Date.now() && (
        <p>{t("sidebar:controlRestore.retryElapsed")}</p>
      )}
      <p>
        {t("sidebar:controlRestore.lastFailureAt")}:{" "}
        {healthTime(row.last_failure) ?? t("common:unknown")}
      </p>
    </div>
  );
}

function healthTime(at: string | undefined) {
  return parseTurnTimestamp(at) === null ? undefined : formatDateTime(at!);
}
