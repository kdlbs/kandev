import { useTranslation } from "react-i18next";
import { useOptionalAppStore } from "@/components/state-provider";
import type { DynamicAgentCandidate, DynamicAgentSelection } from "@/lib/types/agent-profile";
import { settingsControlClassName } from "./settings-control";

function modelOptions(value: unknown): Record<string, unknown> | null {
  if (value === undefined) return {};
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

export function withTierSelection(
  candidate: DynamicAgentCandidate,
  field: "mode" | "on_failure",
  value: string,
): DynamicAgentCandidate {
  const selection = candidate.policies.selection;
  return {
    ...candidate,
    policies: {
      ...candidate.policies,
      selection: {
        ...selection,
        tier: { ...selection?.tier, [field]: value },
      },
    },
  };
}

export function withCandidateCost(
  candidate: DynamicAgentCandidate,
  cost: string,
): DynamicAgentCandidate {
  const prior = candidate.policies.selection;
  const model = modelOptions(prior?.model);
  if (!model) return candidate;
  // The server merges omitted fields from the saved selection, so Unknown is
  // sent as an explicit null to clear a saved cost.
  const next = { ...model, cost: cost || null };
  const selection: DynamicAgentSelection = { ...prior, model: next };
  return { ...candidate, policies: { ...candidate.policies, selection } };
}

const USAGE_TIER_MODES = ["headroom", "pace", "consumption_rate"];

function SelectionField({
  label,
  value,
  options,
  disabled,
  unavailable = [],
  onChange,
}: {
  label: string;
  value: string;
  options: Array<[string, string]>;
  disabled?: boolean;
  unavailable?: string[];
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <label className="grid min-w-0 gap-1 text-xs text-muted-foreground">
      {label}
      <select
        className={settingsControlClassName("w-full")}
        aria-label={label}
        value={value}
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
      >
        {options.map(([key, text]) => (
          <option key={key} value={key} disabled={unavailable.includes(key)}>
            {text}
          </option>
        ))}
        {!options.some(([key]) => key === value) && (
          <option value={value}>{t("agents:tierStoredUnsupported", { value })}</option>
        )}
      </select>
    </label>
  );
}

function TierHeadFields({
  candidate,
  onChange,
}: {
  candidate: DynamicAgentCandidate;
  onChange: (candidate: DynamicAgentCandidate) => void;
}) {
  const { t } = useTranslation();
  const selection = candidate.policies.selection;
  const mode = selection?.tier?.mode ?? "order";
  const usageAvailable = useOptionalAppStore(
    (state) => state.features?.providerUsageLimits ?? false,
    false,
  );
  return (
    <>
      <SelectionField
        label={t("agents:tierSelectionMode")}
        value={mode}
        options={[
          ["order", t("agents:tierModeOrder")],
          ["cost", t("agents:tierModeCost")],
          ["random", t("agents:tierModeRandom")],
          ["round_robin", t("agents:tierModeRoundRobin")],
          ["headroom", t("agents:tierModeHeadroom")],
          ["pace", t("agents:tierModePace")],
          ["consumption_rate", t("agents:tierModeConsumptionRate")],
        ]}
        unavailable={usageAvailable ? [] : USAGE_TIER_MODES}
        onChange={(value) => onChange(withTierSelection(candidate, "mode", value))}
      />
      <SelectionField
        label={t("agents:tierFailureDirection")}
        value={selection?.tier?.on_failure ?? "same_tier_next"}
        options={[
          ["same_tier_next", t("agents:tierSameTierNext")],
          ["next_tier", t("agents:tierNextTier")],
        ]}
        onChange={(value) => onChange(withTierSelection(candidate, "on_failure", value))}
      />
      {mode === "round_robin" && (
        <p className="text-xs text-muted-foreground sm:col-span-2">
          {t("agents:tierRoundRobinDescription")}
        </p>
      )}
      {usageAvailable && USAGE_TIER_MODES.includes(mode) && (
        <p className="text-xs text-muted-foreground sm:col-span-2">
          {t("agents:tierUsageUnknownHint")}
        </p>
      )}
      {!usageAvailable && (
        <p className="text-xs text-muted-foreground sm:col-span-2">
          {t("agents:tierUsageModesUnavailable")}
        </p>
      )}
    </>
  );
}

export function DynamicTierSelectionControls({
  candidate,
  head,
  onChange,
}: {
  candidate: DynamicAgentCandidate;
  head: boolean;
  onChange: (candidate: DynamicAgentCandidate) => void;
}) {
  const { t } = useTranslation();
  const selection = candidate.policies.selection;
  const model = modelOptions(selection?.model);
  const cost = typeof model?.cost === "string" ? model.cost : "";
  return (
    <div
      className="grid min-w-0 gap-3 sm:grid-cols-2"
      data-testid={`tier-selection-${candidate.executionProfileId}`}
    >
      {head && <TierHeadFields candidate={candidate} onChange={onChange} />}
      <SelectionField
        label={t("agents:tierConfiguredCost")}
        value={cost}
        disabled={!model}
        options={[
          ["", t("agents:tierCostUnknown")],
          ["free", t("agents:tierCostFree")],
          ["subscription", t("agents:tierCostSubscription")],
          ["metered", t("agents:tierCostMetered")],
        ]}
        onChange={(value) => onChange(withCandidateCost(candidate, value))}
      />
    </div>
  );
}
