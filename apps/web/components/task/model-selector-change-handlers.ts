"use client";

import { useCallback, useRef, type MutableRefObject } from "react";
import { useTranslation } from "react-i18next";

import { isModelConfigOption, type SelectConfigOption } from "@/components/model-config-selector";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { t } from "@/lib/i18n";
import { setSessionConfigOption, setSessionModel } from "@/lib/api/domains/session-api";
import {
  applyModelConfiguration,
  pendingConfigurationOptions,
} from "@/lib/apply-model-configuration";
import type {
  ConfigOptionEntry,
  SessionModelEntry,
} from "@/lib/state/slices/session-runtime/types";

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

function describeError(err: unknown): string {
  return err instanceof Error ? err.message : t("task:unknownError2");
}

type RequestIds = MutableRefObject<Record<string, number>>;

type ConfigurationApplyOptions = {
  configOptions: SelectConfigOption[];
  sessionModelsData: SessionModelsEntry | undefined;
  activeModels: Record<string, string>;
  latestReqId: RequestIds;
  nextReqId: (sessionId: string) => number;
  updateLocalConfig: (sessionId: string, configId: string, value: string) => void;
  setActiveModel: (sessionId: string, modelId: string) => void;
  setSessionModels: (sessionId: string, data: SessionModelsEntry) => void;
};

function createSessionMutator({
  sid,
  modelConfig,
  store,
  executionId,
  latestReqId,
  reqId,
}: {
  sid: string;
  modelConfig: SelectConfigOption | undefined;
  store: ReturnType<typeof useAppStoreApi>;
  executionId: string | undefined;
  latestReqId: RequestIds;
  reqId: number;
}) {
  const confirmedExecutionIsCurrent = () => {
    const live = store.getState().sessionModels.bySessionId[sid];
    if (!live) return false;
    if (!executionId) return true;
    // A missing live id means the execution was invalidated while the
    // confirmation was pending; only the exact captured execution is current.
    return live.confirmedConfigOptionsExecutionId === executionId;
  };
  return {
    isCurrent: () => latestReqId.current[sid] === reqId && confirmedExecutionIsCurrent(),
    applyModel: async (modelId: string) => {
      if (modelConfig) {
        await setSessionConfigOption(sid, modelConfig.id, modelId);
        return;
      }
      await setSessionModel(sid, modelId);
    },
    applyOption: async (optionId: string, value: string) => {
      await setSessionConfigOption(sid, optionId, value);
    },
  };
}

function handleModelFailure({
  sid,
  previousActive,
  previousModels,
  t,
  toast,
  setActiveModel,
  setSessionModels,
}: {
  sid: string;
  previousActive: string;
  previousModels: SessionModelsEntry | undefined;
  t: (key: string) => string;
  toast: ReturnType<typeof useToast>["toast"];
  setActiveModel: (sessionId: string, modelId: string) => void;
  setSessionModels: (sessionId: string, data: SessionModelsEntry) => void;
}) {
  console.error("[ModelSelector] model change failed");
  setActiveModel(sid, previousActive);
  if (previousModels) setSessionModels(sid, previousModels);
  toast({
    title: t("task:failedToChangeModel"),
    description: t("task:unknownError2"),
    variant: "error",
  });
}

async function retryConfigurationOptions({
  model,
  pending,
  applyModel,
  applyOption,
  updateLocalConfig,
  sid,
  isCurrent,
  t,
  toast,
}: {
  model: string;
  pending: Record<string, string>;
  applyModel: (model: string) => Promise<void>;
  applyOption: (optionId: string, value: string) => Promise<void>;
  updateLocalConfig: (sid: string, configId: string, value: string) => void;
  sid: string;
  isCurrent: () => boolean;
  t: (key: string, options?: Record<string, unknown>) => string;
  toast: ReturnType<typeof useToast>["toast"];
}): Promise<void> {
  const retried = await applyModelConfiguration({
    model,
    configOptions: pending,
    modelConfirmed: true,
    applyModel,
    applyOption,
    onOptionConfirmed: (optionId, value) => updateLocalConfig(sid, optionId, value),
    isCurrent,
  });
  if (retried.status !== "option_failed") return;
  notifyPartialConfiguration({
    result: retried,
    requested: pending,
    t,
    toast,
    retry: (nextPending) =>
      void retryConfigurationOptions({
        model,
        pending: nextPending,
        applyModel,
        applyOption,
        updateLocalConfig,
        sid,
        isCurrent,
        t,
        toast,
      }),
  });
}

function notifyPartialConfiguration({
  result,
  requested,
  t,
  toast,
  retry,
}: {
  result: Extract<Awaited<ReturnType<typeof applyModelConfiguration>>, { status: "option_failed" }>;
  requested: Record<string, string> | undefined;
  t: (key: string, options?: Record<string, unknown>) => string;
  toast: ReturnType<typeof useToast>["toast"];
  retry: (pending: Record<string, string>) => void;
}) {
  const pending = pendingConfigurationOptions(result, requested);
  toast({
    title: t("task:modelConfigurationPartialTitle"),
    description: t("task:modelConfigurationPartialDescription", {
      option: result.failedOptionId,
    }),
    variant: "error",
    action: { label: t("task:modelConfigurationPartialRetry"), onClick: () => retry(pending) },
  });
}

/**
 * Applies one completed model configuration to a live session through the
 * provider-confirmed mutation path. It never optimistically publishes option
 * values: each accepted mutation updates the owning snapshot, and a later
 * failure keeps the confirmed state and offers a retry from there.
 */
function useSessionConfigurationApply({
  configOptions,
  sessionModelsData,
  activeModels,
  latestReqId,
  nextReqId,
  updateLocalConfig,
  setActiveModel,
  setSessionModels,
}: ConfigurationApplyOptions) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const store = useAppStoreApi();

  return useCallback(
    async (sid: string, selection: { model: string; configOptions: Record<string, string> }) => {
      const reqId = nextReqId(sid);
      const previousActive = activeModels[sid] ?? "";
      const previousModels = sessionModelsData;
      const executionId = sessionModelsData?.confirmedConfigOptionsExecutionId;
      const modelConfig = configOptions.find(isModelConfigOption);
      const { isCurrent, applyModel, applyOption } = createSessionMutator({
        sid,
        modelConfig,
        store,
        executionId,
        latestReqId,
        reqId,
      });

      const result = await applyModelConfiguration({
        model: selection.model,
        configOptions: selection.configOptions,
        applyModel,
        applyOption,
        onModelConfirmed: (modelId) => {
          setActiveModel(sid, modelId);
          if (modelConfig) {
            updateLocalConfig(sid, modelConfig.id, modelId);
            return;
          }
          if (sessionModelsData) {
            setSessionModels(sid, {
              ...sessionModelsData,
              currentModelId: modelId,
              fallbackModel: undefined,
            });
          }
        },
        onOptionConfirmed: (optionId, value) => updateLocalConfig(sid, optionId, value),
        isCurrent,
      });

      if (result.status === "superseded" || result.status === "cancelled") return;
      if (result.status === "model_failed") {
        handleModelFailure({
          sid,
          previousActive,
          previousModels,
          t,
          toast,
          setActiveModel,
          setSessionModels,
        });
        return;
      }
      if (result.status === "option_failed") {
        notifyPartialConfiguration({
          result,
          requested: selection.configOptions ?? {},
          t,
          toast,
          retry: (pending) =>
            void retryConfigurationOptions({
              model: selection.model,
              pending,
              applyModel,
              applyOption,
              updateLocalConfig,
              sid,
              isCurrent,
              t,
              toast,
            }),
        });
      }
    },
    [
      activeModels,
      configOptions,
      latestReqId,
      nextReqId,
      sessionModelsData,
      setActiveModel,
      setSessionModels,
      store,
      t,
      toast,
      updateLocalConfig,
    ],
  );
}

/** Builds model/config change handlers with optimistic update + error toast + revert. */
export function useModelChangeHandlers(
  configOptions: SelectConfigOption[],
  sessionModelsData: SessionModelsEntry | undefined,
) {
  const activeModels = useAppStore((state) => state.activeModel.bySessionId);
  const setActiveModel = useAppStore((state) => state.setActiveModel);
  const setSessionModels = useAppStore((state) => state.setSessionModels);
  const store = useAppStoreApi();
  const { toast } = useToast();
  // Per-session monotonic request id so a stale failure doesn't clobber a
  // newer successful selection (rapid A -> B -> C where B fails).
  const latestReqId = useRef<Record<string, number>>({});

  const updateLocalConfig = useCallback(
    (sid: string, configId: string, value: string) => {
      // Each confirmation starts from the live entry so earlier confirmed
      // values and newer provider updates are never overwritten by the
      // pre-Apply snapshot.
      const live = store.getState().sessionModels.bySessionId[sid] ?? sessionModelsData;
      if (!live) return;
      // A manual model change ends the fallback story — drop the note so the
      // picker stops labelling the model as a fallback.
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
    (
      sid: string,
      reqId: number,
      previousActive: string,
      previousModels: SessionModelsEntry | undefined,
    ) =>
      (err: unknown) => {
        if (latestReqId.current[sid] !== reqId) return;
        console.error("[ModelSelector] model change failed:", err);
        setActiveModel(sid, previousActive);
        if (previousModels) setSessionModels(sid, previousModels);
        toast({
          title: t("task:failedToChangeModel"),
          description: describeError(err),
          variant: "error",
        });
      },
    [setActiveModel, setSessionModels, toast],
  );

  const nextReqId = useCallback((sid: string) => {
    const id = (latestReqId.current[sid] ?? 0) + 1;
    latestReqId.current[sid] = id;
    return id;
  }, []);

  const handleModelChange = useCallback(
    (sid: string, modelId: string) => {
      const reqId = nextReqId(sid);
      const fail = onFail(sid, reqId, activeModels[sid] ?? "", sessionModelsData);
      setActiveModel(sid, modelId);
      const modelConfig = configOptions.find(isModelConfigOption);
      if (modelConfig) {
        updateLocalConfig(sid, modelConfig.id, modelId);
        setSessionConfigOption(sid, modelConfig.id, modelId).catch(fail);
        return;
      }
      // Non-config path: the next models_updated event would preserve the
      // fallback note (see session-models handler) — clear it locally now
      // that the user picked a model explicitly.
      if (sessionModelsData) {
        setSessionModels(sid, { ...sessionModelsData, fallbackModel: undefined });
      }
      setSessionModel(sid, modelId).catch(fail);
    },
    [
      activeModels,
      configOptions,
      nextReqId,
      onFail,
      sessionModelsData,
      setActiveModel,
      updateLocalConfig,
    ],
  );

  const handleConfigChange = useCallback(
    (sid: string, configId: string, value: string) => {
      const reqId = nextReqId(sid);
      const fail = onFail(sid, reqId, activeModels[sid] ?? "", sessionModelsData);
      updateLocalConfig(sid, configId, value);
      setSessionConfigOption(sid, configId, value).catch(fail);
    },
    [activeModels, nextReqId, onFail, sessionModelsData, updateLocalConfig],
  );

  const handleConfigurationApply = useSessionConfigurationApply({
    configOptions,
    sessionModelsData,
    activeModels,
    latestReqId,
    nextReqId,
    updateLocalConfig,
    setActiveModel,
    setSessionModels,
  });

  return { handleModelChange, handleConfigChange, handleConfigurationApply };
}
