import { ApiError } from "@/lib/api/client";
import type {
  SessionConfigurationResult,
  SetSessionConfigurationResponse,
} from "@/lib/api/domains/session-api";
import {
  applyModelConfiguration,
  pendingConfigurationOptions,
} from "@/lib/apply-model-configuration";
import type {
  ConfigurationChangeOutcome,
  PickerConfigurationFeedbackController,
} from "./session-configuration-feedback";

export function errorConfigurationResult(error: unknown): SessionConfigurationResult | undefined {
  if (!(error instanceof ApiError) || !error.body || typeof error.body !== "object") {
    return undefined;
  }
  const result = (error.body as { configuration_result?: unknown }).configuration_result;
  return result && typeof result === "object" ? (result as SessionConfigurationResult) : undefined;
}

type RetryConfigurationOptions = {
  model: string;
  feedback: PickerConfigurationFeedbackController;
  modelLabel: string;
  optionLabels: Map<string, string>;
  applyModel: (model: string) => Promise<SetSessionConfigurationResponse>;
  applyOption: (optionId: string, value: string) => Promise<SetSessionConfigurationResponse>;
  updateLocalConfig: (sid: string, configId: string, value: string) => void;
  sid: string;
  isCurrent: () => boolean;
  isConfirmedResponse: (response: unknown) => boolean;
};

type RetryAttempt = {
  result: Awaited<ReturnType<typeof applyModelConfiguration>>;
  changes: ConfigurationChangeOutcome[];
  error?: unknown;
};

async function runRetrySteps(
  options: RetryConfigurationOptions,
  pending: Record<string, string>,
): Promise<RetryAttempt> {
  const changes: ConfigurationChangeOutcome[] = [];
  let failure: unknown;
  const result = await applyModelConfiguration({
    model: options.model,
    configOptions: pending,
    modelConfirmed: true,
    applyModel: options.applyModel,
    applyOption: async (optionId, value) => {
      try {
        const response = await options.applyOption(optionId, value);
        changes.push({
          configId: optionId,
          label: options.optionLabels.get(optionId) ?? optionId,
          requested: value,
          result: response.configuration_result,
        });
        return response;
      } catch (error) {
        failure = error;
        changes.push({
          configId: optionId,
          label: options.optionLabels.get(optionId) ?? optionId,
          requested: value,
          result: errorConfigurationResult(error),
        });
        throw error;
      }
    },
    onOptionConfirmed: (optionId, value) => options.updateLocalConfig(options.sid, optionId, value),
    onOptionSaved: (optionId, value) => options.updateLocalConfig(options.sid, optionId, value),
    isCurrent: options.isCurrent,
    isConfirmedResponse: options.isConfirmedResponse,
  });
  return { result, changes, error: failure };
}

function mergeConfigurationChanges(
  previous: ConfigurationChangeOutcome[],
  next: ConfigurationChangeOutcome[],
): ConfigurationChangeOutcome[] {
  const byId = new Map(previous.map((change) => [change.configId ?? change.label, change]));
  next.forEach((change) => byId.set(change.configId ?? change.label, change));
  return [...byId.values()];
}

function isExplicitRejection(result: ConfigurationChangeOutcome | undefined, error: unknown) {
  return (
    result?.result?.status === "rejected" || errorConfigurationResult(error)?.status === "rejected"
  );
}

type RetryFeedbackOptions = RetryConfigurationOptions & {
  requestId: number;
  pending: Record<string, string>;
  previousChanges: ConfigurationChangeOutcome[];
};

function retryAction(
  options: RetryFeedbackOptions,
  pending: Record<string, string>,
  changes: ConfigurationChangeOutcome[],
) {
  return () => {
    const requestId = options.feedback.begin();
    void retryConfigurationOptions({ ...options, pending, previousChanges: changes, requestId });
  };
}

function showRetryFailure(
  options: RetryFeedbackOptions,
  attempt: RetryAttempt,
  changes: ConfigurationChangeOutcome[],
) {
  const result = attempt.result;
  if (result.status !== "option_failed") return;
  const failed = attempt.changes.find((change) => change.configId === result.failedOptionId);
  const pending = pendingConfigurationOptions(result, options.pending);
  options.feedback.showChanges(
    options.requestId,
    changes,
    {
      label: options.optionLabels.get(result.failedOptionId) ?? result.failedOptionId,
      value: result.failedValue,
      error: attempt.error,
    },
    isExplicitRejection(failed, attempt.error) ? retryAction(options, pending, changes) : undefined,
  );
}

export async function retryConfigurationOptions({
  pending,
  previousChanges,
  requestId,
  ...options
}: RetryConfigurationOptions & {
  pending: Record<string, string>;
  previousChanges: ConfigurationChangeOutcome[];
  requestId: number;
}): Promise<void> {
  const attempt = await runRetrySteps(options, pending);
  if (attempt.result.status === "cancelled" || attempt.result.status === "superseded") return;
  const changes = mergeConfigurationChanges(previousChanges, attempt.changes);
  const feedbackOptions = { ...options, requestId, pending, previousChanges: changes };
  if (attempt.result.status === "option_failed") {
    showRetryFailure(feedbackOptions, attempt, changes);
    return;
  }
  const persistenceFailure = errorConfigurationResult(attempt.error);
  options.feedback.showChanges(
    requestId,
    changes,
    persistenceFailure?.status === "persistence_failed"
      ? {
          label: persistenceFailure.config_id ?? options.modelLabel,
          value: persistenceFailure.effective ?? options.model,
          error: attempt.error,
        }
      : undefined,
  );
}
