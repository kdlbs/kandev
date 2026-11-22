"use client";

import { useLayoutEffect, useMemo, useRef, type RefObject } from "react";
import { ApiError } from "@/lib/api/client";
import type { SessionConfigurationResult } from "@/lib/api/domains/session-api";
import { useAppStoreApi } from "@/components/state-provider";
import { t } from "@/lib/i18n";
import { isPassthroughSession } from "@/lib/session/is-passthrough-session";
import { sanitizeSessionErrorDetails } from "@/lib/session-error-details";
import {
  configurationFeedbackTurnId,
  type ConfigurationFeedbackEntry,
  type ConfigurationFeedbackOutcome,
  type SessionConfigurationFeedbackUpdate,
} from "@/lib/state/slices/session-runtime/session-configuration-feedback";
import type { AppState } from "@/lib/state/store";
import { sessionConfigurationExecutionId } from "./session-configuration-request";

export type ConfigurationChangeOutcome = {
  configId: string;
  label: string;
  requested: string;
  result?: SessionConfigurationResult;
  error?: unknown;
};

export type PickerConfigurationFeedbackController = {
  begin: () => number;
  showChanges: (id: number, changes: ConfigurationChangeOutcome[], retry?: () => void) => void;
};

type FeedbackRequest = {
  id: number;
  sessionId: string;
  executionId?: string;
  turnId: string | null;
  createdAt: string;
};

type FeedbackStore = ReturnType<typeof useAppStoreApi>;

export function errorConfigurationResult(error: unknown): SessionConfigurationResult | undefined {
  if (!(error instanceof ApiError) || !error.body || typeof error.body !== "object") {
    return undefined;
  }
  const result = (error.body as { configuration_result?: unknown }).configuration_result;
  return result && typeof result === "object" ? (result as SessionConfigurationResult) : undefined;
}

function advertisedValueName(
  state: AppState,
  sessionId: string,
  kind: "mode" | "model",
  value: string,
) {
  const profileId = state.taskSessions.items[sessionId]?.agent_profile_id;
  const agent = state.settingsAgents.items.find((item) =>
    item.profiles.some((profile) => profile.id === profileId),
  );
  const config = state.availableAgents.items.find(
    (item) => item.name === agent?.name,
  )?.model_config;
  const options = kind === "mode" ? config?.available_modes : config?.available_models;
  return options?.find((option) => option.id === value)?.name || value;
}

function modeValueName(state: AppState, sessionId: string, value: string) {
  const modes = state.sessionMode.bySessionId[sessionId]?.availableModes;
  return (
    modes?.find((mode) => mode.id === value)?.name ||
    advertisedValueName(state, sessionId, "mode", value)
  );
}

function valueName(state: AppState, sessionId: string, configId: string, value: string) {
  if (configId === "mode") return modeValueName(state, sessionId, value);
  const models = state.sessionModels.bySessionId[sessionId];
  const option = models?.configOptions.find((item) => item.id === configId);
  const optionName = option?.options?.find((item) => item.value === value)?.name;
  if (optionName) return optionName;
  if (configId !== "model" && option?.category !== "model") return value;
  return (
    models?.models.find((model) => model.modelId === value)?.name ||
    advertisedValueName(state, sessionId, "model", value)
  );
}

function feedbackOutcome({
  change,
  result,
  requested,
  effective,
}: {
  change: ConfigurationChangeOutcome;
  result: SessionConfigurationResult | undefined;
  requested: string;
  effective: string | undefined;
}): ConfigurationFeedbackOutcome {
  const reason = sanitizeSessionErrorDetails(change.error);
  const label = (change.configId === "mode" ? t("common:mode") : change.label).toLowerCase();
  const withLabel = (value: string) => `${change.label}: ${value}`;
  const requestedSetting = withLabel(requested);
  switch (result?.status) {
    case "applied":
      if (effective) {
        if (result.effective === result.requested) {
          return { kind: "changed", label, value: effective };
        }
        return {
          kind: "notice",
          text: t("task:sessionConfigurationClamped", {
            requested: requestedSetting,
            effective: withLabel(effective),
          }),
          dismissible: false,
        };
      }
      break;
    case "saved_for_launch":
      return { kind: "saved", label, value: requested };
    case "persistence_failed":
      return {
        kind: "notice",
        text: effective
          ? t("task:sessionConfigurationPersistenceFailed", { value: withLabel(effective), reason })
          : t("task:sessionConfigurationSaveFailed", { value: requestedSetting, reason }),
        dismissible: true,
      };
  }
  if (
    result?.status === "rejected" ||
    (change.error instanceof ApiError && change.error.status < 500)
  ) {
    return {
      kind: "notice",
      text: t("task:sessionConfigurationRejected", {
        value: requestedSetting,
        reason: reason || t("task:sessionConfigurationRejectedReason"),
      }),
      dismissible: true,
    };
  }
  return { kind: "unconfirmed", label, value: requested, reason };
}

function feedbackEntry(
  state: AppState,
  sessionId: string,
  executionId: string | undefined,
  change: ConfigurationChangeOutcome,
): ConfigurationFeedbackEntry {
  const result = change.result ?? errorConfigurationResult(change.error);
  const effective =
    executionId && result?.execution_id === executionId && result.confirmed && result.effective
      ? valueName(state, sessionId, change.configId, result.effective)
      : undefined;
  return {
    configId: change.configId,
    ...feedbackOutcome({
      change,
      result,
      requested: valueName(state, sessionId, change.configId, change.requested),
      effective,
    }),
  };
}

function requestMatchesExecution(
  request: FeedbackRequest,
  current: string | undefined,
  changes: ConfigurationChangeOutcome[],
) {
  const resultIds = changes.flatMap((change) => {
    const executionId = (change.result ?? errorConfigurationResult(change.error))?.execution_id;
    return executionId ? [executionId] : [];
  });
  if (request.executionId === current) {
    return !current || resultIds.every((id) => id === current);
  }
  return (
    !request.executionId &&
    !!current &&
    resultIds.length > 0 &&
    resultIds.every((id) => id === current)
  );
}

function createFeedbackController(
  store: FeedbackStore,
  owner: RefObject<string | null>,
  source: "model" | "mode",
): PickerConfigurationFeedbackController {
  let latestId = 0;
  let request: FeedbackRequest | undefined;
  const begin = () => {
    const id = ++latestId;
    const sessionId = owner.current;
    if (!sessionId) {
      request = undefined;
      return id;
    }
    const state = store.getState();
    const turnId = configurationFeedbackTurnId(state.turns.bySession[sessionId]);
    const executionId = sessionConfigurationExecutionId(state, sessionId);
    const createdAt = new Date().toISOString();
    request = {
      id,
      sessionId,
      executionId,
      turnId,
      createdAt,
    };
    if (source === "model") {
      state.recordSessionConfigurationFeedback(sessionId, {
        turnId,
        executionId,
        createdAt,
        entries: [],
        retry: null,
      });
    }
    return id;
  };

  const showChanges: PickerConfigurationFeedbackController["showChanges"] = (
    id,
    changes,
    retry,
  ) => {
    if (!request || id !== request.id || request.sessionId !== owner.current) return;
    const { sessionId, turnId, createdAt } = request;
    const state = store.getState();
    const session = state.taskSessions.items[sessionId];
    if (
      !session ||
      isPassthroughSession(session) ||
      state.quickChat.sessions.some((item) => item.sessionId === sessionId)
    )
      return;
    const executionId = sessionConfigurationExecutionId(state, sessionId);
    if (!requestMatchesExecution(request, executionId, changes)) return;
    const currentTurnId = configurationFeedbackTurnId(state.turns.bySession[sessionId]);
    let retryAction: SessionConfigurationFeedbackUpdate["retry"];
    if (source === "model") {
      retryAction =
        retry && turnId === currentTurnId && request.executionId === executionId
          ? { run: retry, executionId }
          : null;
    }
    state.recordSessionConfigurationFeedback(sessionId, {
      turnId,
      executionId: request.executionId,
      createdAt,
      entries: changes.map((change) => feedbackEntry(state, sessionId, executionId, change)),
      retry: retryAction,
    });
  };
  return { begin, showChanges };
}

export function usePickerConfigurationFeedback(sessionId: string | null, source: "model" | "mode") {
  const store = useAppStoreApi();
  const owner = useRef(sessionId);
  useLayoutEffect(() => {
    owner.current = sessionId;
  }, [sessionId]);
  return useMemo(() => createFeedbackController(store, owner, source), [store, source]);
}
