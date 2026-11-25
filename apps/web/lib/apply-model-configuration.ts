/**
 * Applies one completed model configuration to a live session.
 *
 * ACP has no atomic transaction. The base model must be confirmed before
 * dependent options can run, and every option is committed from its own
 * correlated result. Saved and unconfirmed outcomes remain visible to callers.
 */
export type ModelConfigurationEvidence = {
  status?: "applied" | "unconfirmed" | "saved_for_launch" | "rejected" | "persistence_failed";
  confirmed?: boolean;
  effective?: string;
  execution_id?: string;
};

type ConfigurationSetterResponse = {
  configuration_result?: ModelConfigurationEvidence;
};

type ApplyResultBase = {
  model: string;
  appliedOptions: Record<string, string>;
  savedOptions?: string[];
  unconfirmedOptions?: string[];
};

export type ModelConfigurationApplyRequest = {
  model: string;
  configOptions?: Record<string, string>;
  /** The model is already confirmed for this generation; start at the options. */
  modelConfirmed?: boolean;
  applyModel: (model: string) => Promise<unknown>;
  applyOption: (optionId: string, value: string) => Promise<unknown>;
  /** Reports each accepted provider mutation so the owning surface stays authoritative. */
  onModelConfirmed?: (model: string) => void;
  onModelSaved?: (model: string) => void;
  onOptionConfirmed?: (optionId: string, value: string) => void;
  onOptionSaved?: (optionId: string, value: string) => void;
  isCurrent?: () => boolean;
  isConfirmedResponse?: (response: unknown) => boolean;
};

export type ModelConfigurationApplyResult =
  | (ApplyResultBase & { status: "applied" })
  | (ApplyResultBase & { status: "model_failed" | "model_unconfirmed" | "model_saved" })
  | (ApplyResultBase & {
      status: "model_clamped";
      requested: string;
      effective: string;
    })
  | (ApplyResultBase & {
      status: "option_failed";
      failedOptionId: string;
      failedValue: string;
    })
  | (ApplyResultBase & {
      status: "superseded" | "cancelled";
    });

type StepOutcome =
  | { status: "applied"; effective: string }
  | { status: "persistence_failed"; effective: string }
  | { status: "saved" }
  | { status: "unconfirmed" }
  | { status: "rejected" };

function stepOutcome(response: unknown, requested: string): StepOutcome {
  const result = configurationResult(response);
  if (!result) return { status: "unconfirmed" };
  switch (result.status) {
    case "saved_for_launch":
      return { status: "saved" };
    case "rejected":
      return { status: "rejected" };
    case "persistence_failed":
      return hasConfirmedEffectiveValue(result)
        ? { status: "persistence_failed", effective: result.effective }
        : { status: "unconfirmed" };
    case "applied":
      return hasConfirmedEffectiveValue(result)
        ? { status: "applied", effective: result.effective || requested }
        : { status: "unconfirmed" };
    default:
      return { status: "unconfirmed" };
  }
}

function configurationResult(response: unknown): ModelConfigurationEvidence | undefined {
  if (!response || typeof response !== "object") return undefined;
  const value = response as ConfigurationSetterResponse & { body?: unknown };
  if (value.configuration_result) return value.configuration_result;
  if (!value.body || typeof value.body !== "object") return undefined;
  return (value.body as ConfigurationSetterResponse).configuration_result;
}

function hasConfirmedEffectiveValue(
  result: ModelConfigurationEvidence,
): result is ModelConfigurationEvidence & { effective: string } {
  return result.confirmed === true && typeof result.effective === "string" && !!result.effective;
}

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
  const savedOptions: string[] = [];
  const unconfirmedOptions: string[] = [];
  const isCurrent = request.isCurrent ?? (() => true);
  const partial = (): ApplyResultBase => ({
    model: request.model,
    appliedOptions: { ...appliedOptions },
    ...(savedOptions.length > 0 ? { savedOptions: [...savedOptions] } : {}),
    ...(unconfirmedOptions.length > 0 ? { unconfirmedOptions: [...unconfirmedOptions] } : {}),
  });

  if (!isCurrent()) return { status: "cancelled", ...partial() };
  if (!request.modelConfirmed) {
    const baseResult = await applyBaseModel(request, isCurrent, partial);
    if (baseResult) return baseResult;
  }

  const options = request.configOptions ?? {};
  for (const optionId of orderedOptionIds(options)) {
    const result = await applyConfigurationOption({
      request,
      optionId,
      requested: options[optionId],
      isCurrent,
      partial,
      appliedOptions,
      savedOptions,
      unconfirmedOptions,
    });
    if (result) return result;
  }

  return { status: "applied", ...partial() };
}

type PartialResult = () => ApplyResultBase;

type OptionApplicationContext = {
  request: ModelConfigurationApplyRequest;
  optionId: string;
  requested: string;
  isCurrent: () => boolean;
  partial: PartialResult;
  appliedOptions: Record<string, string>;
  savedOptions: string[];
  unconfirmedOptions: string[];
};

async function applyBaseModel(
  request: ModelConfigurationApplyRequest,
  isCurrent: () => boolean,
  partial: PartialResult,
): Promise<ModelConfigurationApplyResult | undefined> {
  let response: unknown;
  try {
    response = await request.applyModel(request.model);
  } catch (error) {
    if (!isCurrent()) return { status: "superseded", ...partial() };
    if (stepOutcome(error, request.model).status !== "persistence_failed") {
      return { status: "model_failed", ...partial() };
    }
    response = error;
  }
  if (!isCurrent()) return { status: "superseded", ...partial() };
  const outcome = stepOutcome(response, request.model);
  if (outcome.status === "rejected") return { status: "model_failed", ...partial() };
  if (outcome.status === "unconfirmed") return { status: "model_unconfirmed", ...partial() };
  if (outcome.status === "saved") {
    request.onModelSaved?.(request.model);
    return { status: "model_saved", ...partial() };
  }
  if (request.isConfirmedResponse && !request.isConfirmedResponse(response)) {
    return { status: "model_unconfirmed", ...partial() };
  }
  request.onModelConfirmed?.(outcome.effective);
  if (outcome.effective === request.model) return undefined;
  return {
    status: "model_clamped",
    ...partial(),
    requested: request.model,
    effective: outcome.effective,
  };
}

async function applyConfigurationOption({
  request,
  optionId,
  requested,
  isCurrent,
  partial,
  appliedOptions,
  savedOptions,
  unconfirmedOptions,
}: OptionApplicationContext): Promise<ModelConfigurationApplyResult | undefined> {
  let response: unknown;
  try {
    response = await request.applyOption(optionId, requested);
  } catch (error) {
    if (!isCurrent()) return { status: "superseded", ...partial() };
    if (stepOutcome(error, requested).status !== "persistence_failed") {
      return optionFailure(optionId, requested, partial);
    }
    response = error;
  }
  if (!isCurrent()) return { status: "superseded", ...partial() };
  const outcome = stepOutcome(response, requested);
  if (outcome.status === "rejected") return optionFailure(optionId, requested, partial);
  if (outcome.status === "unconfirmed") {
    unconfirmedOptions.push(optionId);
    return undefined;
  }
  if (outcome.status === "saved") {
    savedOptions.push(optionId);
    request.onOptionSaved?.(optionId, requested);
    return undefined;
  }
  if (request.isConfirmedResponse && !request.isConfirmedResponse(response)) {
    unconfirmedOptions.push(optionId);
    return undefined;
  }
  appliedOptions[optionId] = outcome.effective;
  request.onOptionConfirmed?.(optionId, outcome.effective);
  return undefined;
}

function optionFailure(
  failedOptionId: string,
  failedValue: string,
  partial: PartialResult,
): ModelConfigurationApplyResult {
  return { status: "option_failed", ...partial(), failedOptionId, failedValue };
}

/** Options that still need a confirmation after a partial result. */
export function pendingConfigurationOptions(
  result: ModelConfigurationApplyResult,
  requested: Record<string, string> | undefined,
): Record<string, string> {
  if (
    result.status === "applied" ||
    result.status === "model_saved" ||
    result.status === "cancelled" ||
    result.status === "superseded"
  ) {
    return {};
  }
  const unconfirmed = new Set(result.unconfirmedOptions ?? []);
  const saved = new Set(result.savedOptions ?? []);
  const pending: Record<string, string> = {};
  for (const [optionId, value] of Object.entries(requested ?? {})) {
    if (
      !Object.hasOwn(result.appliedOptions, optionId) &&
      !unconfirmed.has(optionId) &&
      !saved.has(optionId)
    ) {
      pending[optionId] = value;
    }
  }
  return pending;
}
