"use client";

import { useCallback, type MutableRefObject } from "react";
import { useTranslation } from "react-i18next";

import { isModelConfigOption, type SelectConfigOption } from "@/components/model-config-selector";
import { useAppStoreApi } from "@/components/state-provider";
import {
  setSessionConfigOption,
  setSessionModel,
  type SetSessionConfigurationResponse,
} from "@/lib/api/domains/session-api";
import {
  applyModelConfiguration,
  pendingConfigurationOptions,
} from "@/lib/apply-model-configuration";
import type {
  ConfigurationChangeOutcome,
  PickerConfigurationFeedbackController,
} from "./session-configuration-feedback";
import type { SessionModelsEntry } from "./model-selector-change-handlers";
import { createSessionConfigurationRequestFence } from "./session-configuration-request";
import {
  errorConfigurationResult,
  retryConfigurationOptions,
} from "./model-selector-configuration-retry";

type RequestIds = MutableRefObject<Record<string, number>>;

type ConfigurationApplyOptions = {
  configOptions: SelectConfigOption[];
  sessionModelsData: SessionModelsEntry | undefined;
  latestReqId: RequestIds;
  nextReqId: (sessionId: string) => number;
  updateLocalConfig: (sessionId: string, configId: string, value: string) => void;
  setActiveModel: (sessionId: string, modelId: string) => void;
  setSessionModels: (sessionId: string, data: SessionModelsEntry) => void;
  feedback: PickerConfigurationFeedbackController;
};

function createSessionMutator({
  sid,
  modelConfig,
  store,
  latestReqId,
  reqId,
}: {
  sid: string;
  modelConfig: SelectConfigOption | undefined;
  store: ReturnType<typeof useAppStoreApi>;
  latestReqId: RequestIds;
  reqId: number;
}) {
  const fence = createSessionConfigurationRequestFence({
    sessionId: sid,
    store,
    latestRequestIds: latestReqId,
    requestId: reqId,
  });
  const observeResponse = <T>(response: T): T => {
    fence.observeResponse(response);
    return response;
  };
  return {
    ...fence,
    applyModel: async (modelId: string) => {
      try {
        const response = modelConfig
          ? await setSessionConfigOption(sid, modelConfig.id, modelId)
          : await setSessionModel(sid, modelId);
        return observeResponse(response);
      } catch (error) {
        observeResponse(error);
        throw error;
      }
    },
    applyOption: async (optionId: string, value: string, expectedModelId?: string) => {
      try {
        const response = expectedModelId
          ? await setSessionConfigOption(sid, optionId, value, expectedModelId)
          : await setSessionConfigOption(sid, optionId, value);
        return observeResponse(response);
      } catch (error) {
        observeResponse(error);
        throw error;
      }
    },
  };
}

type ModelConfigurationSelection = {
  model: string;
  configOptions: Record<string, string>;
};

type ModelConfigurationRunOptions = ConfigurationApplyOptions & {
  sid: string;
  selection: ModelConfigurationSelection;
  modelLabel: string;
  store: ReturnType<typeof useAppStoreApi>;
};

type ConfigurationFailureCapture = { error?: unknown };

type SessionMutator = ReturnType<typeof createSessionMutator>;

function createModelValueWriter({
  sid,
  modelConfig,
  sessionModelsData,
  store,
  setSessionModels,
  updateLocalConfig,
}: {
  sid: string;
  modelConfig: SelectConfigOption | undefined;
  sessionModelsData: SessionModelsEntry | undefined;
  store: ReturnType<typeof useAppStoreApi>;
  setSessionModels: (sessionId: string, data: SessionModelsEntry) => void;
  updateLocalConfig: (sessionId: string, configId: string, value: string) => void;
}) {
  return (modelId: string, liveChange: boolean) => {
    if (modelConfig) {
      updateLocalConfig(sid, modelConfig.id, modelId);
      return;
    }
    const live = store.getState().sessionModels.bySessionId[sid] ?? sessionModelsData;
    if (!live) return;
    const next = { ...live, currentModelId: modelId };
    if (liveChange) {
      next.fallbackModel = undefined;
      next.savedModelPreferenceId = undefined;
    }
    setSessionModels(sid, next);
  };
}

async function captureConfigurationStep(
  action: () => Promise<SetSessionConfigurationResponse>,
  onSuccess: (response: SetSessionConfigurationResponse) => void,
  failure: ConfigurationFailureCapture,
  onError?: (error: unknown) => void,
) {
  try {
    const response = await action();
    onSuccess(response);
    return response;
  } catch (error) {
    failure.error = error;
    onError?.(error);
    throw error;
  }
}

function executeModelConfigurationSelection({
  sid,
  selection,
  sessionModelsData,
  store,
  modelConfig,
  modelConfigId,
  modelLabel,
  optionLabels,
  changes,
  failure,
  mutator,
  setActiveModel,
  setSessionModels,
  updateLocalConfig,
}: {
  sid: string;
  selection: ModelConfigurationSelection;
  sessionModelsData: SessionModelsEntry | undefined;
  store: ReturnType<typeof useAppStoreApi>;
  modelConfig: SelectConfigOption | undefined;
  modelConfigId: string;
  modelLabel: string;
  optionLabels: Map<string, string>;
  changes: ConfigurationChangeOutcome[];
  failure: ConfigurationFailureCapture;
  mutator: SessionMutator;
  setActiveModel: (sessionId: string, modelId: string) => void;
  setSessionModels: (sessionId: string, data: SessionModelsEntry) => void;
  updateLocalConfig: (sessionId: string, configId: string, value: string) => void;
}) {
  const { isCurrent, canCommit, applyModel, applyOption } = mutator;
  const writeModelValue = createModelValueWriter({
    sid,
    modelConfig,
    sessionModelsData,
    store,
    setSessionModels,
    updateLocalConfig,
  });
  return applyModelConfiguration({
    model: selection.model,
    configOptions: selection.configOptions,
    applyModel: (modelId) =>
      captureConfigurationStep(
        () => applyModel(modelId),
        (response) =>
          changes.push({
            configId: modelConfigId,
            label: modelLabel,
            requested: modelId,
            result: response.configuration_result,
          }),
        failure,
        (error) =>
          changes.push({
            configId: modelConfigId,
            label: modelLabel,
            requested: modelId,
            result: errorConfigurationResult(error),
            error,
          }),
      ),
    applyOption: (optionId, value) =>
      captureConfigurationStep(
        () => applyOption(optionId, value),
        (response) =>
          changes.push({
            configId: optionId,
            label: optionLabels.get(optionId) ?? optionId,
            requested: value,
            result: response.configuration_result,
          }),
        failure,
        (error) =>
          changes.push({
            configId: optionId,
            label: optionLabels.get(optionId) ?? optionId,
            requested: value,
            result: errorConfigurationResult(error),
            error,
          }),
      ),
    onModelConfirmed: (modelId) => {
      setActiveModel(sid, modelId);
      writeModelValue(modelId, true);
    },
    onModelSaved: (modelId) => writeModelValue(modelId, false),
    onOptionConfirmed: (optionId, value) => updateLocalConfig(sid, optionId, value),
    onOptionSaved: (optionId, value) => updateLocalConfig(sid, optionId, value),
    isCurrent,
    isConfirmedResponse: canCommit,
  });
}

function reportModelConfigurationFailure(
  options: ModelConfigurationRunOptions & {
    feedbackId: number;
    changes: ConfigurationChangeOutcome[];
    error: unknown;
  },
) {
  const { selection, feedbackId, error, modelLabel, feedback } = options;
  feedback.showError(feedbackId, modelLabel, selection.model, error);
}

function reportModelConfigurationOptionFailure(
  options: ModelConfigurationRunOptions & {
    result: Awaited<ReturnType<typeof applyModelConfiguration>>;
    feedbackId: number;
    changes: ConfigurationChangeOutcome[];
    error: unknown;
    optionLabels: Map<string, string>;
    applyModel: (model: string) => Promise<SetSessionConfigurationResponse>;
    applyOption: (
      optionId: string,
      value: string,
      expectedModelId?: string,
    ) => Promise<SetSessionConfigurationResponse>;
    isCurrent: () => boolean;
    isConfirmedResponse: (response: unknown) => boolean;
  },
) {
  const {
    sid,
    selection,
    result,
    feedbackId,
    changes,
    error,
    modelLabel,
    feedback,
    optionLabels,
    applyModel,
    applyOption,
    updateLocalConfig,
    isCurrent,
    isConfirmedResponse,
  } = options;
  if (result.status !== "option_failed") return;
  const pending = pendingConfigurationOptions(result, selection.configOptions);
  const failedChange = changes.find((change) => change.configId === result.failedOptionId);
  const canRetry =
    failedChange?.result?.status === "rejected" ||
    errorConfigurationResult(error)?.status === "rejected";
  feedback.showChanges(
    feedbackId,
    changes,
    {
      label: optionLabels.get(result.failedOptionId) ?? result.failedOptionId,
      value: result.failedValue,
      error,
    },
    canRetry
      ? () => {
          const retryId = feedback.begin();
          void retryConfigurationOptions({
            model: selection.model,
            pending,
            previousChanges: changes,
            feedback,
            requestId: retryId,
            modelLabel,
            optionLabels,
            applyModel,
            applyOption,
            updateLocalConfig,
            sid,
            isCurrent,
            isConfirmedResponse,
          });
        }
      : undefined,
  );
}

async function runModelConfigurationSelection(options: ModelConfigurationRunOptions) {
  const {
    sid,
    selection,
    configOptions,
    sessionModelsData,
    latestReqId,
    nextReqId,
    updateLocalConfig,
    setActiveModel,
    setSessionModels,
    feedback,
    store,
    modelLabel,
  } = options;
  const reqId = nextReqId(sid);
  const modelConfig = configOptions.find(isModelConfigOption);
  const modelConfigId = modelConfig?.id ?? "model";
  const optionLabels = new Map(
    configOptions.map((option) => [option.id, option.name || option.id]),
  );
  const feedbackId = feedback.begin();
  const changes: ConfigurationChangeOutcome[] = [];
  const failure: ConfigurationFailureCapture = {};
  const mutator = createSessionMutator({
    sid,
    modelConfig,
    store,
    latestReqId,
    reqId,
  });
  const { isCurrent, applyModel, applyOption } = mutator;
  const result = await executeModelConfigurationSelection({
    sid,
    selection,
    sessionModelsData,
    store,
    modelConfig,
    modelConfigId,
    modelLabel,
    optionLabels,
    changes,
    failure,
    mutator,
    setActiveModel,
    setSessionModels,
    updateLocalConfig,
  });

  if (result.status === "superseded" || result.status === "cancelled") return;
  if (result.status === "model_failed") {
    reportModelConfigurationFailure({
      ...options,
      feedbackId,
      changes,
      error: failure.error,
    });
    return;
  }
  if (result.status === "option_failed") {
    reportModelConfigurationOptionFailure({
      ...options,
      result,
      feedbackId,
      changes,
      error: failure.error,
      optionLabels,
      applyModel,
      applyOption,
      isCurrent,
      isConfirmedResponse: mutator.canCommit,
    });
    return;
  }
  const persistenceError = changes.find((change) => change.result?.status === "persistence_failed");
  feedback.showChanges(
    feedbackId,
    changes,
    persistenceError && failure.error
      ? {
          label: persistenceError.label,
          value: persistenceError.result?.effective ?? persistenceError.requested,
          error: failure.error,
        }
      : undefined,
  );
}

/**
 * Applies one completed model configuration to a live session through the
 * provider-confirmed mutation path. It never optimistically publishes option
 * values: each accepted mutation updates the owning snapshot, and a later
 * failure keeps the confirmed state and offers a retry from there.
 */
export function useSessionConfigurationApply(options: ConfigurationApplyOptions) {
  const {
    configOptions,
    sessionModelsData,
    latestReqId,
    nextReqId,
    updateLocalConfig,
    setActiveModel,
    setSessionModels,
    feedback,
  } = options;
  const { t } = useTranslation();
  const store = useAppStoreApi();
  const modelLabel = configOptions.find(isModelConfigOption)?.name || t("common:model");

  return useCallback(
    (sid: string, selection: ModelConfigurationSelection) =>
      runModelConfigurationSelection({
        sid,
        selection,
        configOptions,
        sessionModelsData,
        latestReqId,
        nextReqId,
        updateLocalConfig,
        setActiveModel,
        setSessionModels,
        feedback,
        modelLabel,
        store,
      }),
    [
      configOptions,
      feedback,
      latestReqId,
      modelLabel,
      nextReqId,
      sessionModelsData,
      setActiveModel,
      setSessionModels,
      store,
      updateLocalConfig,
    ],
  );
}
