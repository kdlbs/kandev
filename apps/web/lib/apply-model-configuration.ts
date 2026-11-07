/**
 * Applies one completed model configuration to a live session.
 *
 * ACP has no atomic transaction: the model is confirmed first, then each
 * requested option is confirmed in order. The result always describes the state
 * the provider actually accepted, so a later failure surfaces a retryable
 * partial application instead of claiming success or restoring an optimistic
 * draft. `isCurrent` fences a superseded session, execution, or selection
 * generation; nothing is confirmed after it reports false.
 */
export type ModelConfigurationApplyRequest = {
  model: string;
  configOptions?: Record<string, string>;
  /** The model is already confirmed for this generation; start at the options. */
  modelConfirmed?: boolean;
  applyModel: (model: string) => Promise<void>;
  applyOption: (optionId: string, value: string) => Promise<void>;
  /** Reports each accepted provider mutation so the owning surface stays authoritative. */
  onModelConfirmed?: (model: string) => void;
  onOptionConfirmed?: (optionId: string, value: string) => void;
  isCurrent?: () => boolean;
};

export type ModelConfigurationApplyResult =
  | { status: "applied"; model: string; appliedOptions: Record<string, string> }
  | { status: "model_failed"; model: string; appliedOptions: Record<string, string> }
  | {
      status: "option_failed";
      model: string;
      appliedOptions: Record<string, string>;
      failedOptionId: string;
      failedValue: string;
    }
  | { status: "superseded"; model: string; appliedOptions: Record<string, string> }
  | { status: "cancelled"; model: string; appliedOptions: Record<string, string> };

/** Sorted option ids keep confirmation order deterministic across callers. */
function orderedOptionIds(configOptions: Record<string, string> | undefined): string[] {
  return Object.keys(configOptions ?? {})
    .filter((optionId) => !!optionId)
    .sort();
}

export async function applyModelConfiguration(
  request: ModelConfigurationApplyRequest,
): Promise<ModelConfigurationApplyResult> {
  const appliedOptions: Record<string, string> = {};
  const isCurrent = request.isCurrent ?? (() => true);
  const partial = () => ({ model: request.model, appliedOptions: { ...appliedOptions } });

  if (!isCurrent()) return { status: "cancelled", ...partial() };
  if (!request.modelConfirmed) {
    try {
      await request.applyModel(request.model);
    } catch {
      if (!isCurrent()) return { status: "superseded", ...partial() };
      return { status: "model_failed", model: request.model, appliedOptions: {} };
    }
    if (!isCurrent()) return { status: "superseded", ...partial() };
    request.onModelConfirmed?.(request.model);
  }

  const options = request.configOptions ?? {};
  for (const optionId of orderedOptionIds(options)) {
    try {
      await request.applyOption(optionId, options[optionId]);
    } catch {
      if (!isCurrent()) return { status: "superseded", ...partial() };
      return {
        status: "option_failed",
        model: request.model,
        appliedOptions: { ...appliedOptions },
        failedOptionId: optionId,
        failedValue: options[optionId],
      };
    }
    if (!isCurrent()) return { status: "superseded", ...partial() };
    appliedOptions[optionId] = options[optionId];
    request.onOptionConfirmed?.(optionId, options[optionId]);
  }

  return { status: "applied", model: request.model, appliedOptions };
}

/** Options that still need a confirmation after a partial result. */
export function pendingConfigurationOptions(
  result: ModelConfigurationApplyResult,
  requested: Record<string, string> | undefined,
): Record<string, string> {
  if (result.status === "applied" || result.status === "cancelled") return {};
  const pending: Record<string, string> = {};
  for (const [optionId, value] of Object.entries(requested ?? {})) {
    if (!Object.hasOwn(result.appliedOptions, optionId)) pending[optionId] = value;
  }
  return pending;
}
