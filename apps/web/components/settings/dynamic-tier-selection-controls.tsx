import { useTranslation } from "react-i18next";
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
  const next = { ...model };
  if (cost) next.cost = cost;
  else delete next.cost;
  const selection: DynamicAgentSelection = { ...prior, model: next };
  return { ...candidate, policies: { ...candidate.policies, selection } };
}

function SelectionField({
  label,
  value,
  options,
  disabled,
  onChange,
}: {
  label: string;
  value: string;
  options: Array<[string, string]>;
  disabled?: boolean;
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
          <option key={key} value={key}>
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
      {head && (
        <>
          <SelectionField
            label={t("agents:tierSelectionMode")}
            value={selection?.tier?.mode ?? "order"}
            options={[
              ["order", t("agents:tierModeOrder")],
              ["cost", t("agents:tierModeCost")],
              ["random", t("agents:tierModeRandom")],
            ]}
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
        </>
      )}
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
