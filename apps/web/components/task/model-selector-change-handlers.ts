"use client";

import { useCallback, useRef, type MutableRefObject } from "react";
import { useTranslation } from "react-i18next";

import { isModelConfigOption, type SelectConfigOption } from "@/components/model-config-selector";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { setSessionConfigOption, setSessionModel } from "@/lib/api/domains/session-api";
import type {
  ConfigOptionEntry,
  SessionModelEntry,
} from "@/lib/state/slices/session-runtime/types";
import {
  errorConfigurationResult,
  type PickerConfigurationFeedbackController,
} from "./session-configuration-feedback";
import { createSessionConfigurationRequestFence } from "./session-configuration-request";
import { useSessionConfigurationApply } from "./model-selector-configuration-apply";

export type SessionModelsEntry = {
  currentModelId: string;
  models: SessionModelEntry[];
  configOptions: ConfigOptionEntry[];
  configOptionsSettled?: boolean;
  configBaseline?: Record<string, string>;
  settingsPolicy?: "provider_restored";
  /** Set when the session started on the profile's fallback model. */
  fallbackModel?: string;
  /** Provider execution that produced the confirmed configuration. */
  confirmedConfigOptionsExecutionId?: string;
  savedModelPreferenceId?: string;
};

function updateConfigOptionValue(
  options: ConfigOptionEntry[],
  configId: string,
  value: string,
): ConfigOptionEntry[] {
  return options.map((option) =>
    option.id === configId ? { ...option, currentValue: value } : option,
  );
}

function nextCurrentModelId(
  data: { currentModelId: string; configOptions: ConfigOptionEntry[] },
  configId: string,
  value: string,
): string {
  const option = data.configOptions.find((item) => item.id === configId);
  if (option && isModelConfigOption(option)) return value;
  return data.currentModelId;
}

type RequestIds = MutableRefObject<Record<string, number>>;

type ChangeFailure = {
  sid: string;
  reqId: number;
  feedbackId: number;
  configId: string;
  label: string;
  requested: string;
  isCurrent: () => boolean;
};

type OnChangeFailure = (failure: ChangeFailure) => (error: unknown) => void;

type SettingChangeHandlerOptions = {
  configOptions: SelectConfigOption[];
  sessionModelsData: SessionModelsEntry | undefined;
  latestReqId: RequestIds;
  nextReqId: (sessionId: string) => number;
  updateLocalConfig: (sessionId: string, configId: string, value: string) => void;
  setActiveModel: (sessionId: string, modelId: string) => void;
  setSessionModels: (sessionId: string, data: SessionModelsEntry) => void;
  feedback: PickerConfigurationFeedbackController;
  store: ReturnType<typeof useAppStoreApi>;
  onFail: OnChangeFailure;
};

type ModelValueUpdateOptions = Pick<
  SettingChangeHandlerOptions,
  "sessionModelsData" | "setActiveModel" | "setSessionModels" | "store" | "updateLocalConfig"
> & {
  sid: string;
  modelConfig: SelectConfigOption | undefined;
};

function commitEffectiveModel(options: ModelValueUpdateOptions, modelId: string) {
  options.setActiveModel(options.sid, modelId);
  if (options.modelConfig) {
    options.updateLocalConfig(options.sid, options.modelConfig.id, modelId);
    return;
  }
  const live =
    options.store.getState().sessionModels.bySessionId[options.sid] ?? options.sessionModelsData;
  if (live) {
    options.setSessionModels(options.sid, {
      ...live,
      currentModelId: modelId,
      savedModelPreferenceId: undefined,
      fallbackModel: undefined,
    });
  }
}

function saveModelPreference(options: ModelValueUpdateOptions, modelId: string) {
  if (options.modelConfig) {
    options.updateLocalConfig(options.sid, options.modelConfig.id, modelId);
    return;
  }
  const live =
    options.store.getState().sessionModels.bySessionId[options.sid] ?? options.sessionModelsData;
  if (live) {
    options.setSessionModels(options.sid, {
      ...live,
      currentModelId: modelId,
      savedModelPreferenceId: modelId,
    });
  }
}

function useModelValueChangeHandler(options: SettingChangeHandlerOptions) {
  const { t } = useTranslation();
  const {
    configOptions,
    sessionModelsData,
    latestReqId,
    nextReqId,
    updateLocalConfig,
    setActiveModel,
    setSessionModels,
    feedback,
    store,
    onFail,
  } = options;

  return useCallback(
    (sid: string, modelId: string) => {
      const reqId = nextReqId(sid);
      const modelConfig = configOptions.find(isModelConfigOption);
      const label = modelConfig?.name || t("common:model");
      const change = { configId: modelConfig?.id ?? "model", label, requested: modelId };
      const feedbackId = feedback.begin();
      const fence = createSessionConfigurationRequestFence({
        sessionId: sid,
        store,
        latestRequestIds: latestReqId,
        requestId: reqId,
      });
      const fail = onFail({
        sid,
        reqId,
        feedbackId,
        ...change,
        isCurrent: fence.isCurrent,
      });
      const apply = modelConfig
        ? setSessionConfigOption(sid, modelConfig.id, modelId)
        : setSessionModel(sid, modelId);
      const updateOptions: ModelValueUpdateOptions = {
        sid,
        modelConfig,
        sessionModelsData,
        setActiveModel,
        setSessionModels,
        store,
        updateLocalConfig,
      };

      void apply
        .then((response) => {
          if (!fence.observeResponse(response)) return;
          const outcome = response.configuration_result;
          if (fence.canCommit(response) && outcome?.effective) {
            commitEffectiveModel(updateOptions, outcome.effective);
          } else if (fence.isCurrent() && outcome?.status === "saved_for_launch") {
            saveModelPreference(updateOptions, modelId);
          }
          feedback.showChanges(feedbackId, [{ ...change, result: outcome }]);
        })
        .catch((error: unknown) => {
          fence.observeResponse(error);
          const outcome = errorConfigurationResult(error);
          if (fence.canCommit(error) && outcome?.effective) {
            commitEffectiveModel(updateOptions, outcome.effective);
          }
          fail(error);
        });
    },
    [
      configOptions,
      feedback,
      latestReqId,
      nextReqId,
      onFail,
      sessionModelsData,
      setActiveModel,
      setSessionModels,
      store,
      t,
      updateLocalConfig,
    ],
  );
}

function useConfigOptionChangeHandler(options: SettingChangeHandlerOptions) {
  const { configOptions, latestReqId, nextReqId, updateLocalConfig, feedback, store, onFail } =
    options;

  return useCallback(
    (sid: string, configId: string, value: string) => {
      const reqId = nextReqId(sid);
      const option = configOptions.find((item) => item.id === configId);
      const label = option?.name || configId;
      const change = { configId, label, requested: value };
      const feedbackId = feedback.begin();
      const fence = createSessionConfigurationRequestFence({
        sessionId: sid,
        store,
        latestRequestIds: latestReqId,
        requestId: reqId,
      });
      const fail = onFail({
        sid,
        reqId,
        feedbackId,
        ...change,
        isCurrent: fence.isCurrent,
      });

      void setSessionConfigOption(sid, configId, value)
        .then((response) => {
          if (!fence.observeResponse(response)) return;
          const outcome = response.configuration_result;
          if (fence.canCommit(response) && outcome?.effective) {
            updateLocalConfig(sid, configId, outcome.effective);
          } else if (fence.isCurrent() && outcome?.status === "saved_for_launch") {
            updateLocalConfig(sid, configId, value);
          }
          feedback.showChanges(feedbackId, [{ ...change, result: outcome }]);
        })
        .catch((error: unknown) => {
          fence.observeResponse(error);
          const outcome = errorConfigurationResult(error);
          if (fence.canCommit(error) && outcome?.effective) {
            updateLocalConfig(sid, configId, outcome.effective);
          }
          fail(error);
        });
    },
    [configOptions, feedback, latestReqId, nextReqId, onFail, store, updateLocalConfig],
  );
}

/** Builds model/config change handlers with correlated inline outcomes. */
export function useModelChangeHandlers(
  configOptions: SelectConfigOption[],
  sessionModelsData: SessionModelsEntry | undefined,
  feedback: PickerConfigurationFeedbackController,
) {
  const setActiveModel = useAppStore((state) => state.setActiveModel);
  const setSessionModels = useAppStore((state) => state.setSessionModels);
  const store = useAppStoreApi();
  // Each session owns a monotonic request id so an older action cannot replace
  // a newer result while its provider request is still pending.
  const latestReqId = useRef<Record<string, number>>({});

  const updateLocalConfig = useCallback(
    (sid: string, configId: string, value: string) => {
      const live = store.getState().sessionModels.bySessionId[sid] ?? sessionModelsData;
      if (!live) return;
      const fallbackModel =
        configId === live.configOptions.find(isModelConfigOption)?.id
          ? undefined
          : live.fallbackModel;
      setSessionModels(sid, {
        ...live,
        fallbackModel,
        currentModelId: nextCurrentModelId(live, configId, value),
        configOptions: updateConfigOptionValue(live.configOptions, configId, value),
      });
    },
    [sessionModelsData, setSessionModels, store],
  );

  const onFail = useCallback(
    ({ sid, reqId, feedbackId, configId, label, requested, isCurrent }: ChangeFailure) =>
      (error: unknown) => {
        if (latestReqId.current[sid] !== reqId || !isCurrent()) return;
        feedback.showChanges(feedbackId, [{ configId, label, requested, error }]);
      },
    [feedback],
  );

  const nextReqId = useCallback((sid: string) => {
    const id = (latestReqId.current[sid] ?? 0) + 1;
    latestReqId.current[sid] = id;
    return id;
  }, []);

  const changeHandlerOptions: SettingChangeHandlerOptions = {
    configOptions,
    sessionModelsData,
    latestReqId,
    nextReqId,
    updateLocalConfig,
    setActiveModel,
    setSessionModels,
    feedback,
    store,
    onFail,
  };
  const handleModelChange = useModelValueChangeHandler(changeHandlerOptions);
  const handleConfigChange = useConfigOptionChangeHandler(changeHandlerOptions);

  const handleConfigurationApply = useSessionConfigurationApply({
    configOptions,
    sessionModelsData,
    latestReqId,
    nextReqId,
    updateLocalConfig,
    setActiveModel,
    setSessionModels,
    feedback,
  });

  return { handleModelChange, handleConfigChange, handleConfigurationApply };
}
