import { useTranslation } from "react-i18next";
import { useNativeHistory } from "@/hooks/domains/coordinator/use-control-native";
import { nativeEvents } from "@/lib/control-ui/native-control";
import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import { formatDateTime } from "@/lib/i18n/formats";
import { useIntegratedControl } from "./integrated-control-provider";
import { ControlReadState } from "./control-read-state";
import { OverviewLast24h } from "./control-history-list";

/** Routine office records that say nothing happened; listed only on request. */
const QUIET_KINDS = ["run_idle_skipped"];

export function ControlHistory({ workspaceId }: { workspaceId?: string }) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const workspaces = (control?.data?.workspaces ?? []).filter(
    (w) => !workspaceId || w.id === workspaceId,
  );
  const names = Object.fromEntries(workspaces.map((w) => [w.id, w.name]));
  const result = useNativeHistory(
    workspaces.map((w) => w.id),
    control?.data,
  );
  const events = (result.data ?? [])
    .flatMap((w) => (w.data ? nativeEvents(w.data) : []))
    .sort((a, b) => {
      const x = parseStrictRfc3339Timestamp(a.at)!,
        y = parseStrictRfc3339Timestamp(b.at)!;
      if (x !== y) return x > y ? -1 : 1;
      return `${a.workspace_id}:${a.id}`.localeCompare(`${b.workspace_id}:${b.id}`);
    });
  return (
    <section
      className="space-y-3"
      data-testid="control-history"
      aria-busy={result.status === "loading" || Boolean(result.refreshing)}
    >
      {(result.status !== "ready" || result.stale) && <ControlReadState {...result} />}
      {result.data?.some((w) => Boolean(w.data)) && (
        <OverviewLast24h
          events={events}
          workspaceNames={names}
          emptyLabel={t("sidebar:controlNative.noSupportedEvents")}
          quietKinds={QUIET_KINDS}
        />
      )}
      {result.data
        ?.filter((w) => w.status !== "ready")
        .map((w) => (
          <div key={w.id} className="text-xs text-muted-foreground">
            <span>{names[w.id]}</span>
            <ControlReadState {...w} />
          </div>
        ))}
      <details className="text-xs text-muted-foreground" data-testid="control-history-metadata">
        <summary className="min-h-7 cursor-pointer [@media(pointer:coarse)]:min-h-11">
          {t("sidebar:controlNative.sourceCoverage")}
        </summary>
        {result.data?.map((w) => (
          <div key={w.id}>
            {w.data && (
              <details
                className="text-xs text-muted-foreground"
                data-testid="control-history-coverage"
              >
                <summary className="min-h-7 cursor-pointer [@media(pointer:coarse)]:min-h-11">
                  {names[w.id]} · {formatDateTime(w.data.generated_at)} (
                  {Intl.DateTimeFormat().resolvedOptions().timeZone}) · {w.data.coverage} ·{" "}
                  {t("sidebar:controlNative.sourceCoverage")}
                </summary>
                <p>{t("sidebar:controlNative.historyCoverage")}</p>
                <dl className="grid grid-cols-[minmax(0,1fr)_auto] gap-1">
                  {Object.entries(w.data.sources).map(([name, s]) => (
                    <div key={name} className="contents">
                      <dt className="break-words">{name}</dt>
                      <dd>
                        {s.state === "known"
                          ? `${s.returned}/${s.total ?? t("common:unknown")}`
                          : t("common:unknown")}{" "}
                        {s.reason} {s.truncated && t("sidebar:controlNative.truncated")}
                      </dd>
                    </div>
                  ))}
                </dl>
                {w.data.truncated && <p>{t("sidebar:controlNative.truncated")}</p>}
              </details>
            )}
          </div>
        ))}
      </details>
    </section>
  );
}
