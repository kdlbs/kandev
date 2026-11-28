import { useTranslation } from "react-i18next";
import type { ControlTaskCount } from "@/hooks/domains/coordinator/control-conversation-lists";
import { ControlReadState } from "./control-read-state";

/** Native fallback counts describe only the returned task sample, never the global population. */
export function ControlWorkspaceSample({ list }: { list?: ControlTaskCount }) {
  const { t } = useTranslation();
  if (!list || list.error) return <ControlReadState status={list?.denied ? "denied" : "error"} />;
  const tasks = list.items ?? [];
  const statesKnown = tasks.every((task) => task.primary_session_state != null);
  const stateCount = (state: string) =>
    statesKnown
      ? tasks.filter((task) => task.primary_session_state === state).length
      : t("common:unknown");
  const figures = [
    ["sidebar:controlRestore.processRunning", stateCount("RUNNING")],
    ["sidebar:controlRestore.waitingInput", stateCount("WAITING_FOR_INPUT")],
    ["sidebar:controlRestore.processFailed", stateCount("FAILED")],
    [
      "sidebar:controlRestore.generatingObserved",
      tasks.some((task) => task.foreground_activity === "generating")
        ? tasks.filter((task) => task.foreground_activity === "generating").length
        : t("common:unknown"),
    ],
  ] as const;
  return (
    <section className="space-y-2 px-4 pb-3" data-testid="control-workspace-sample">
      <h3 className="text-sm font-medium">{t("sidebar:controlRestore.nativeSample")}</h3>
      <p className="text-xs text-muted-foreground">
        {t("sidebar:controlRestore.nativeSampleScope", {
          shown: tasks.length,
          total: list.total ?? t("common:unknown"),
        })}
      </p>
      <dl className="grid grid-cols-2 gap-2 lg:grid-cols-4">
        {figures.map(([key, count]) => (
          <div key={key} className="rounded-md border p-2 text-center">
            <dt className="text-xs text-muted-foreground">{t(key)}</dt>
            <dd className="text-lg font-semibold tabular-nums">{count}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
}
