import { useTranslation } from "react-i18next";
import { formatDateTime } from "@/lib/i18n/formats";
import {
  CIRCUIT_CODE_KEYS,
  CIRCUIT_SCOPE_KEYS,
  CIRCUIT_STATE_KEYS,
  blockingCircuitCount,
  isCircuitBlocking,
  shortCircuitId,
  sortCircuits,
  type RoutingCircuit,
} from "@/lib/control-ui/routing-circuits";
import { useControlRoutingCircuits } from "@/hooks/domains/coordinator/use-control-circuits";
import { useIntegratedControl } from "./integrated-control-provider";

const READ_STATUS_KEYS = {
  loading: "common:loading",
  denied: "sidebar:integratedControl.readDenied",
  unsupported: "sidebar:controlRestore.contractUnavailable",
  error: "sidebar:integratedControl.loadError",
};

/**
 * Shared provider resource circuits, separate from the workspace routing health
 * (`office_provider_health`) shown above: this table is installation-wide
 * because a credential circuit is shared by every workspace that uses it.
 */
export function ControlProviderCircuits() {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const result = useControlRoutingCircuits(
    control?.data?.workspaces.length ? control.data : undefined,
  );
  if (!control?.data || control.data.workspaces.length === 0) return null;
  const circuits = sortCircuits(result.data ?? []);
  const blocking = blockingCircuitCount(circuits);
  return (
    <section className="rounded-lg border" aria-label={t("sidebar:controlCircuits.title")}>
      <header className="flex flex-wrap items-center justify-between gap-2 p-4">
        <h2 className="font-medium">{t("sidebar:controlCircuits.title")}</h2>
        <span className="text-xs text-muted-foreground">
          {t("sidebar:controlCircuits.installationWide")}
        </span>
      </header>
      <CircuitReadState result={result} />
      {result.status === "ready" && circuits.length === 0 && (
        <p className="px-4 py-4 text-sm text-muted-foreground">
          {t("sidebar:controlCircuits.empty")}
        </p>
      )}
      {circuits.length > 0 && (
        <>
          {blocking > 0 && (
            <p className="px-4 py-2 text-sm">
              {t("sidebar:controlCircuits.blocking", { count: blocking })}
            </p>
          )}
          <ul className="divide-y border-t">
            {circuits.map((circuit) => (
              <CircuitRow key={circuit.resource_key} circuit={circuit} />
            ))}
          </ul>
        </>
      )}
    </section>
  );
}

function CircuitReadState({ result }: { result: ReturnType<typeof useControlRoutingCircuits> }) {
  const { t } = useTranslation();
  if (result.status === "ready") return null;
  return (
    <p
      role={result.status === "error" || result.status === "denied" ? "alert" : "status"}
      className="px-4 pb-4 text-sm text-muted-foreground"
    >
      {result.stale && result.receivedAt
        ? t("sidebar:controlRefresh.retainedAt", { time: formatDateTime(result.receivedAt) })
        : t(READ_STATUS_KEYS[result.status])}
    </p>
  );
}

function CircuitRow({ circuit }: { circuit: RoutingCircuit }) {
  const { t } = useTranslation();
  const scope = CIRCUIT_SCOPE_KEYS[circuit.scope];
  const code =
    circuit.code && Object.hasOwn(CIRCUIT_CODE_KEYS, circuit.code)
      ? CIRCUIT_CODE_KEYS[circuit.code]
      : "common:unknownError";
  const blocking = isCircuitBlocking(circuit);
  const stateKey =
    circuit.state === "open" && !blocking
      ? "sidebar:controlCircuits.state.available"
      : CIRCUIT_STATE_KEYS[circuit.state];
  const candidateDue = circuit.state === "half_open" ? circuit.probe_until : circuit.until;
  const due = blocking ? candidateDue : undefined;
  return (
    <li
      className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2 text-sm"
      data-testid="control-provider-circuit"
    >
      <span className="rounded border px-1.5 py-0.5 text-[11px]">{t(stateKey)}</span>
      <span className="font-medium">
        {scope ? t(scope) : circuit.scope || t("sidebar:controlCircuits.scope.unknown")}
      </span>
      <span className="font-mono text-xs text-muted-foreground">
        {shortCircuitId(circuit.scope_value)}
      </span>
      <span>{t(code)}</span>
      {due && (
        <span className="text-xs text-muted-foreground">
          {t("sidebar:controlCircuits.until", { time: formatDateTime(due) })}
        </span>
      )}
      {circuit.failures > 0 && (
        <span className="text-xs">
          {t("sidebar:controlCircuits.failures", { count: circuit.failures })}
        </span>
      )}
      {circuit.updated_at && (
        <span className="text-xs text-muted-foreground">
          {t("sidebar:controlCircuits.lastRecorded", {
            time: formatDateTime(circuit.updated_at),
          })}
        </span>
      )}
    </li>
  );
}
