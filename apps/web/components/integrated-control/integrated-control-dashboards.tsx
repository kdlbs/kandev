import { useTranslation } from "react-i18next";
import { formatDateTime } from "@/lib/i18n/formats";
import Link from "@/components/routing/app-link";
import { controlHref, type ControlView } from "@/lib/navigation/integrated-control";
import { hasScope, SCOPE } from "@/lib/types/team-access";
import { useIntegratedControl } from "./integrated-control-provider";
import {
  ControlAggregateOverview,
  ControlInboxAttentionFallback,
} from "./control-aggregate-overview";
import { ControlGroupedHistory } from "./control-grouped-history";
import { ControlNativeComparison } from "./control-native-comparison";
import { ControlProviderUsage } from "./control-provider-usage";
import { ControlOverview } from "./control-overview";
import { ControlHistory } from "./control-history";
import { ControlWorkspaces } from "./control-workspaces";
import { ControlBlockerChips } from "./control-blocker-chips";
import { ControlTaskBoard } from "./control-task-board";
import { useControlSignals } from "./use-control-signals";
import { ControlCapacityLine } from "./control-capacity-bars";
import { capacityDashboardCounts } from "./control-capacity-waiting";
import { useNativeDashboards } from "@/hooks/domains/coordinator/use-control-native";
import type { SessionCapacityObservation } from "@/lib/types/system";
import { ControlInbox } from "./control-inbox";

export function IntegratedControlDashboard({
  view,
  workspaceId,
}: {
  view: ControlView;
  workspaceId?: string;
}) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  if (!control?.data) return null;
  const { workspaces, byWorkspace } = control.data;
  const rows = workspaces.flatMap((workspace) =>
    (byWorkspace[workspace.id]?.items ?? []).map((coordinator) => ({ workspace, coordinator })),
  );
  return (
    <div
      className="min-h-0 flex-1 space-y-5 overflow-y-auto p-4 md:p-6"
      data-testid={`control-dashboard-${view}`}
    >
      <ControlInboxView view={view} officeEnabled={control.officeEnabled} />
      <ControlCapacityFallback
        view={view}
        officeEnabled={control.officeEnabled}
        observation={control.data.capacity}
        lastConfirmed={control.data.lastConfirmedCapacity}
        workspaces={workspaces}
        revision={control.data}
      />
      <ControlSignalView
        view={view}
        workspaceId={workspaceId}
        hasWorkspaces={workspaces.length > 0}
      />
      {view === "overview" && (
        <div className="grid gap-3 sm:grid-cols-3">
          <Fact label={t("sidebar:integratedControl.workspaces")} value={workspaces.length} />
          <Fact
            label={t("sidebar:integratedControl.coordinators")}
            value={
              control.coordinatorEnabled ? rows.length : t("sidebar:integratedControl.unavailable")
            }
          />
          <Fact
            label={t("sidebar:integratedControl.currentConversations")}
            value={
              control.coordinatorEnabled
                ? rows.filter((row) => row.coordinator.conversation_task_id).length
                : t("sidebar:integratedControl.unavailable")
            }
          />
        </div>
      )}
      {view === "overview" && control.officeEnabled && (
        <>
          <ControlAggregateOverview
            aggregate={control.aggregate}
            controlWorkspaceIds={workspaces.map((workspace) => workspace.id)}
          />
          <ControlNativeComparison workspaces={workspaces} revision={control.data} />
        </>
      )}
      {view === "history" && control.officeEnabled && (
        <ControlGroupedHistory aggregate={control.aggregate} />
      )}
      <WorkspaceOrCoordinatorFacts
        view={view}
        workspaceId={workspaceId}
        workspaces={workspaces}
        byWorkspace={byWorkspace}
        rows={rows}
      />
      {!control.coordinatorEnabled && (
        <p role="status" className="text-sm text-muted-foreground">
          {t("sidebar:integratedControl.coordinatorsOff")}
        </p>
      )}
      {Object.values(byWorkspace).some((list) => list.error) && (
        <p role="alert" className="text-sm text-destructive">
          {t("sidebar:integratedControl.partialData")}
        </p>
      )}
      {workspaces.length === 0 && (
        <p className="text-sm text-muted-foreground">
          {t("sidebar:integratedControl.noWorkspaces")}
        </p>
      )}
    </div>
  );
}

/** The recorded-signal section each view adds above its existing dashboards. */
function ControlSignalView({
  view,
  workspaceId,
  hasWorkspaces,
}: {
  view: ControlView;
  workspaceId?: string;
  hasWorkspaces: boolean;
}) {
  if (view === "activity") return <ControlActivityBoard />;
  if (view === "providers")
    return hasWorkspaces ? <ControlProviderUsage workspaceId={workspaceId} /> : null;
  if (view === "overview") return <ControlOverview />;
  if (view === "history") return <ControlHistory workspaceId={workspaceId} />;
  if (view === "workspaces" && workspaceId) return <ControlWorkspaces workspaceId={workspaceId} />;
  return null;
}

/** Stopped-work chips and the ranked task board, from the shared signal reads. */
function ControlActivityBoard() {
  const signals = useControlSignals();
  return (
    <>
      <ControlBlockerChips />
      <ControlTaskBoard signals={signals} />
    </>
  );
}

function ControlCapacityFallback({
  view,
  officeEnabled,
  observation,
  lastConfirmed,
  workspaces,
  revision,
}: {
  view: ControlView;
  officeEnabled: boolean;
  observation?: SessionCapacityObservation;
  lastConfirmed?: SessionCapacityObservation;
  workspaces: FactWorkspaces;
  revision: object;
}) {
  const active = view === "activity" && officeEnabled;
  const ids = active ? workspaces.map((workspace) => workspace.id) : [];
  const dashboards = useNativeDashboards(ids, active ? revision : undefined);
  const { wipWaiting, sharedWaiting } = capacityDashboardCounts(ids, dashboards.data);
  if (view !== "activity" && (view !== "overview" || officeEnabled)) return null;
  return (
    <ControlCapacityLine
      observation={observation}
      lastConfirmed={lastConfirmed}
      wipWaiting={active ? wipWaiting : null}
      sharedWaiting={sharedWaiting}
    />
  );
}

function ControlInboxView({ view, officeEnabled }: { view: ControlView; officeEnabled: boolean }) {
  if (view === "inbox") return <ControlInbox />;
  if (view === "overview" && !officeEnabled) return <ControlInboxAttentionFallback />;
  return null;
}

function Fact({ label, value }: { label: string; value: number | string }) {
  return (
    <section className="rounded-lg border p-4">
      <h2 className="text-sm text-muted-foreground">{label}</h2>
      <p className="mt-2 text-2xl font-semibold">{value}</p>
    </section>
  );
}

type FactWorkspaces = NonNullable<
  NonNullable<ReturnType<typeof useIntegratedControl>>["data"]
>["workspaces"];

type FactByWorkspace = NonNullable<
  NonNullable<ReturnType<typeof useIntegratedControl>>["data"]
>["byWorkspace"];

function WorkspaceOrCoordinatorFacts({
  view,
  workspaceId,
  workspaces,
  byWorkspace,
  rows,
}: {
  view: ControlView;
  workspaceId?: string;
  workspaces: FactWorkspaces;
  byWorkspace: FactByWorkspace;
  rows: FactRows;
}) {
  const { t } = useTranslation();
  if (workspaceId) return null;
  if (view === "workspaces" || view === "overview") {
    return (
      <div className="grid gap-3">
        {workspaces.map((workspace) => (
          <section key={workspace.id} className="rounded-lg border p-4">
            <h2 className="font-medium">
              <Link
                className="inline-flex min-h-7 cursor-pointer items-center underline-offset-4 hover:underline max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
                href={controlHref("workspaces", workspace.id)}
              >
                {workspace.name}
              </Link>
            </h2>
            {workspace.description && (
              <p className="mt-1 whitespace-pre-wrap text-sm text-muted-foreground">
                {workspace.description}
              </p>
            )}
            <p className="mt-2 text-xs text-muted-foreground">
              {t(
                hasScope(workspace.scopes, SCOPE.workspaceManage)
                  ? "sidebar:integratedControl.manageAccess"
                  : "sidebar:integratedControl.readAccess",
              )}
            </p>
            {byWorkspace[workspace.id]?.error && (
              <p role="alert" className="mt-2 text-sm text-destructive">
                {t("sidebar:integratedControl.loadError")}
              </p>
            )}
          </section>
        ))}
      </div>
    );
  }
  return <CoordinatorFacts view={view} rows={rows} />;
}

type FactRows = Array<{
  workspace: NonNullable<
    NonNullable<ReturnType<typeof useIntegratedControl>>["data"]
  >["workspaces"][number];
  coordinator: NonNullable<
    NonNullable<ReturnType<typeof useIntegratedControl>>["data"]
  >["byWorkspace"][string]["items"][number];
}>;

function CoordinatorFacts({ view, rows }: { view: ControlView; rows: FactRows }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      {view === "history" && (
        <h2 className="text-sm font-medium">
          {t("sidebar:integratedControl.currentConversations")}
        </h2>
      )}
      {rows.map(({ workspace, coordinator }) => (
        <section key={coordinator.id} className="space-y-2 rounded-lg border p-4">
          <h2 className="font-medium">
            {workspace.name} / {coordinator.name}
          </h2>
          {view === "activity" && (
            <dl className="grid gap-1 text-sm">
              <div>
                <dt className="inline text-muted-foreground">
                  {t("sidebar:integratedControl.pendingProposals")}:{" "}
                </dt>
                <dd className="inline">
                  {coordinator.open_proposals ?? t("sidebar:integratedControl.unknown")}
                </dd>
              </div>
              <div>
                <dt className="inline text-muted-foreground">
                  {t("sidebar:integratedControl.updated")}:{" "}
                </dt>
                <dd className="inline">
                  <time dateTime={coordinator.updated_at}>
                    {formatDateTime(coordinator.updated_at)}
                  </time>
                </dd>
              </div>
            </dl>
          )}
          {view === "providers" && (
            <dl className="space-y-1 break-all text-sm">
              <div>
                <dt className="inline text-muted-foreground">
                  {t("sidebar:integratedControl.agentProfile")}:{" "}
                </dt>
                <dd className="inline">{coordinator.agent_profile_id}</dd>
              </div>
              <div>
                <dt className="inline text-muted-foreground">
                  {t("sidebar:integratedControl.executorProfile")}:{" "}
                </dt>
                <dd className="inline">{coordinator.executor_profile_id}</dd>
              </div>
              <div>
                <dt className="inline text-muted-foreground">
                  {t("sidebar:integratedControl.views.providers")}:{" "}
                </dt>
                <dd className="inline">{t("sidebar:integratedControl.unknown")}</dd>
              </div>
            </dl>
          )}
          {coordinator.conversation_task_id ? (
            <Link
              className="inline-flex min-h-7 cursor-pointer items-center gap-2 rounded text-sm underline underline-offset-4 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
              href={controlHref(view, workspace.id, coordinator.id)}
            >
              {t("sidebar:integratedControl.currentConversation")}
              <span className="break-all text-xs text-muted-foreground">
                {coordinator.conversation_task_id}
              </span>
            </Link>
          ) : (
            <p className="text-sm text-muted-foreground">
              {t("sidebar:integratedControl.noConversation")}
            </p>
          )}
        </section>
      ))}
      {rows.length === 0 && (
        <p className="text-sm text-muted-foreground">
          {t("sidebar:integratedControl.noCoordinators")}
        </p>
      )}
    </div>
  );
}
