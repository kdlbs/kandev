import type { SetSessionConfigurationResponse } from "@/lib/api/domains/session-api";
import {
  applyModelConfiguration,
  pendingConfigurationOptions,
} from "@/lib/apply-model-configuration";
import {
  errorConfigurationResult,
  type ConfigurationChangeOutcome,
  type PickerConfigurationFeedbackController,
} from "./session-configuration-feedback";

type RetryConfigurationOptions = {
  model: string;
  pending: Record<string, string>;
  requestId: number;
  feedback: PickerConfigurationFeedbackController;
  optionLabels: Map<string, string>;
  applyModel: (model: string) => Promise<SetSessionConfigurationResponse>;
  applyOption: (
    optionId: string,
    value: string,
    expectedModelId?: string,
  ) => Promise<SetSessionConfigurationResponse>;
  updateLocalConfig: (sid: string, configId: string, value: string) => void;
  sid: string;
  isCurrent: () => boolean;
  isConfirmedResponse: (response: unknown) => boolean;
};

function applyRetryOptions(
  options: RetryConfigurationOptions,
  changes: ConfigurationChangeOutcome[],
) {
  return applyModelConfiguration({
    model: options.model,
    configOptions: options.pending,
    modelConfirmed: true,
    applyModel: options.applyModel,
    applyOption: async (configId, requested) => {
      const setting = {
        configId,
        label: options.optionLabels.get(configId) ?? configId,
        requested,
      };
      try {
        const response = await options.applyOption(configId, requested, options.model);
        changes.push({ ...setting, result: response.configuration_result });
        return response;
      } catch (error) {
        changes.push({ ...setting, result: errorConfigurationResult(error), error });
        throw error;
      }
    },
    onOptionConfirmed: (optionId, value) => options.updateLocalConfig(options.sid, optionId, value),
    onOptionSaved: (optionId, value) => options.updateLocalConfig(options.sid, optionId, value),
    isCurrent: options.isCurrent,
    isConfirmedResponse: options.isConfirmedResponse,
  });
}

export async function retryConfigurationOptions(options: RetryConfigurationOptions) {
  const changes: ConfigurationChangeOutcome[] = [];
  const result = await applyRetryOptions(options, changes);
  if (result.status === "cancelled" || result.status === "superseded") return;
  const canRetry =
    result.status === "option_failed" &&
    changes.some(
      (change) => change.configId === result.failedOptionId && change.result?.status === "rejected",
    );
  options.feedback.showChanges(
    options.requestId,
    changes,
    canRetry
      ? () => {
          void retryConfigurationOptions({
            ...options,
            requestId: options.feedback.begin(),
            pending: pendingConfigurationOptions(result, options.pending),
          });
        }
      : undefined,
  );
}
