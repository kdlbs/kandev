"use client";

import { useCallback, useRef, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "@/lib/api/client";
import {
  listQuickTerminalTabs,
  toQuickTerminalTab,
  updateQuickTerminalTab,
} from "@/lib/api/domains/quick-terminal-api";
import type { PtyTerminalState } from "@/components/settings/pty-terminal-view";
import type { QuickTerminalTab, QuickTerminalUpdate } from "@/lib/state/slices/ui/types";

export type QuickTerminalRecoveryPhase = "preparing" | "starting";

export type QuickTerminalRecoveryState = {
  generation: number;
  phase?: QuickTerminalRecoveryPhase;
  closing?: boolean;
};

type QuickTerminalLifecycleStore = {
  terminalTabs: QuickTerminalTab[];
  updateQuickTerminal: (tabId: string, update: QuickTerminalUpdate) => void;
};

type RecoveryAttempt = {
  generation: number;
  cancelled: boolean;
  promise: Promise<void>;
};

type RecoveryStateSetter = Dispatch<SetStateAction<Record<string, QuickTerminalRecoveryState>>>;

type QuickTerminalLifecycleContext = {
  workspaceId: string;
  unknownDescriptorError: string;
  storeRef: { current: QuickTerminalLifecycleStore };
  generationByTab: { current: Record<string, number> };
  closingByTab: { current: Record<string, boolean> };
  attemptsByTab: { current: Map<string, RecoveryAttempt> };
  writesByTab: { current: Map<string, Promise<void>> };
  setRecoveryState: RecoveryStateSetter;
};

type QuickTerminalStateCallback = (
  tabId: string,
  state: PtyTerminalState,
  generation: number,
) => void;

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function isCurrentGeneration(
  context: QuickTerminalLifecycleContext,
  tabId: string,
  generation: number,
) {
  return (context.generationByTab.current[tabId] ?? 0) === generation;
}

function canAcceptLifecycleCallback(
  context: QuickTerminalLifecycleContext,
  tabId: string,
  generation: number,
) {
  return !context.closingByTab.current[tabId] && isCurrentGeneration(context, tabId, generation);
}

function updateRecoveryState(
  context: QuickTerminalLifecycleContext,
  tabId: string,
  recovery: QuickTerminalRecoveryState | undefined,
) {
  context.setRecoveryState((current) => {
    if (!recovery && !current[tabId]) return current;
    if (!recovery) {
      const { [tabId]: _removed, ...remaining } = current;
      return remaining;
    }
    return { ...current, [tabId]: recovery };
  });
}

function finishAttempt(
  context: QuickTerminalLifecycleContext,
  tabId: string,
  attempt: RecoveryAttempt,
) {
  if (context.attemptsByTab.current.get(tabId) !== attempt) return;
  context.attemptsByTab.current.delete(tabId);
  updateRecoveryState(context, tabId, { generation: attempt.generation });
}

function applyTerminalDescriptor(
  context: QuickTerminalLifecycleContext,
  tabId: string,
  descriptor: QuickTerminalTab,
) {
  context.storeRef.current.updateQuickTerminal(tabId, {
    sequence: descriptor.sequence,
    sessionId: descriptor.sessionId,
    status: descriptor.status,
    exitCode: descriptor.exitCode,
    error: descriptor.error,
  });
}

function persistTerminalState(
  context: QuickTerminalLifecycleContext,
  tabId: string,
  state: PtyTerminalState,
  generation: number,
) {
  if (!canAcceptLifecycleCallback(context, tabId, generation)) return;
  context.storeRef.current.updateQuickTerminal(tabId, {
    sessionId: state.sessionId,
    status: state.status,
    exitCode: state.exitCode,
    error: state.error,
  });

  const previous = context.writesByTab.current.get(tabId) ?? Promise.resolve();
  const write = previous
    .catch(() => undefined)
    .then(async () => {
      try {
        const descriptor = await updateQuickTerminalTab(tabId, {
          sessionId: state.status === "exited" ? null : state.sessionId,
          status: state.status,
          exitCode: state.exitCode,
          error: state.error,
        });
        if (canAcceptLifecycleCallback(context, tabId, generation)) {
          applyTerminalDescriptor(context, tabId, toQuickTerminalTab(descriptor));
        }
      } catch (error: unknown) {
        if (error instanceof ApiError && error.status === 404) return;
        if (!canAcceptLifecycleCallback(context, tabId, generation)) return;
        context.storeRef.current.updateQuickTerminal(tabId, {
          status: "error",
          error: errorMessage(error),
        });
      }
    });
  context.writesByTab.current.set(tabId, write);
  void write.then(() => {
    if (context.writesByTab.current.get(tabId) === write) {
      context.writesByTab.current.delete(tabId);
    }
  });

  const attempt = context.attemptsByTab.current.get(tabId);
  if (attempt?.generation === generation && state.status !== "connecting") {
    finishAttempt(context, tabId, attempt);
  }
}

async function prepareRecovery(
  context: QuickTerminalLifecycleContext,
  tabId: string,
  attempt: RecoveryAttempt,
) {
  try {
    await context.writesByTab.current.get(tabId)?.catch(() => undefined);
    if (attempt.cancelled || context.attemptsByTab.current.get(tabId) !== attempt) return;

    const { tabs } = await listQuickTerminalTabs(context.workspaceId);
    if (attempt.cancelled || context.attemptsByTab.current.get(tabId) !== attempt) return;
    const descriptor = tabs.find((candidate) => candidate.tabId === tabId);
    if (!descriptor) throw new Error(context.unknownDescriptorError);

    const terminal = toQuickTerminalTab(descriptor);
    if (terminal.status === "running" && terminal.sessionId) {
      applyTerminalDescriptor(context, tabId, terminal);
      finishAttempt(context, tabId, attempt);
      return;
    }

    updateRecoveryState(context, tabId, {
      generation: attempt.generation,
      phase: "starting",
    });
    context.storeRef.current.updateQuickTerminal(tabId, {
      status: "connecting",
      sessionId: null,
      exitCode: null,
      error: null,
    });
  } catch (error: unknown) {
    if (attempt.cancelled || context.attemptsByTab.current.get(tabId) !== attempt) return;
    context.storeRef.current.updateQuickTerminal(tabId, {
      status: "error",
      sessionId: null,
      error: errorMessage(error),
    });
    finishAttempt(context, tabId, attempt);
  }
}

function startRecovery(context: QuickTerminalLifecycleContext, tabId: string): Promise<void> {
  if (context.closingByTab.current[tabId]) return Promise.resolve();
  const existing = context.attemptsByTab.current.get(tabId);
  if (existing) return existing.promise;

  const tab = context.storeRef.current.terminalTabs.find(
    (candidate) => candidate.tabId === tabId && candidate.workspaceId === context.workspaceId,
  );
  if (!tab) return Promise.resolve();

  const generation = (context.generationByTab.current[tabId] ?? 0) + 1;
  context.generationByTab.current[tabId] = generation;
  const attempt: RecoveryAttempt = { generation, cancelled: false, promise: Promise.resolve() };
  context.attemptsByTab.current.set(tabId, attempt);
  updateRecoveryState(context, tabId, { generation, phase: "preparing" });
  context.storeRef.current.updateQuickTerminal(tabId, {
    status: "connecting",
    sessionId: null,
    exitCode: null,
    error: null,
  });

  attempt.promise = prepareRecovery(context, tabId, attempt);
  return attempt.promise;
}

function beginClose(context: QuickTerminalLifecycleContext, tabId: string) {
  if (context.closingByTab.current[tabId]) return false;
  const attempt = context.attemptsByTab.current.get(tabId);
  if (attempt) {
    attempt.cancelled = true;
    context.attemptsByTab.current.delete(tabId);
  }
  context.closingByTab.current[tabId] = true;
  updateRecoveryState(context, tabId, {
    generation: context.generationByTab.current[tabId] ?? 0,
    closing: true,
  });
  return true;
}

function finishClose(context: QuickTerminalLifecycleContext, tabId: string, deleted: boolean) {
  const generation = context.generationByTab.current[tabId] ?? 0;
  if (deleted) {
    const retiredGeneration = generation + 1;
    context.generationByTab.current[tabId] = retiredGeneration;
    updateRecoveryState(context, tabId, { generation: retiredGeneration, closing: true });
    return;
  }

  context.closingByTab.current[tabId] = false;
  updateRecoveryState(context, tabId, { generation });
}

export function useQuickTerminalLifecycle(workspaceId: string, store: QuickTerminalLifecycleStore) {
  const { t } = useTranslation();
  const storeRef = useRef(store);
  storeRef.current = store;
  const generationByTab = useRef<Record<string, number>>({});
  const closingByTab = useRef<Record<string, boolean>>({});
  const attemptsByTab = useRef(new Map<string, RecoveryAttempt>());
  const writesByTab = useRef(new Map<string, Promise<void>>());
  const [terminalRecoveryByTab, setTerminalRecoveryByTab] = useState<
    Record<string, QuickTerminalRecoveryState>
  >({});
  const contextRef = useRef<QuickTerminalLifecycleContext | null>(null);
  contextRef.current = {
    workspaceId,
    unknownDescriptorError: t("sidebar:quickChatTerminalUnknownError"),
    storeRef,
    generationByTab,
    closingByTab,
    attemptsByTab,
    writesByTab,
    setRecoveryState: setTerminalRecoveryByTab,
  };

  const handleTerminalStateChange: QuickTerminalStateCallback = useCallback(
    (tabId, state, generation) => {
      if (contextRef.current) {
        persistTerminalState(contextRef.current, tabId, state, generation);
      }
    },
    [],
  );
  const handleTerminalDescriptorReady = useCallback(
    (tabId: string, descriptor: QuickTerminalTab, generation: number) => {
      const context = contextRef.current;
      if (!context || !canAcceptLifecycleCallback(context, tabId, generation)) return;
      applyTerminalDescriptor(context, tabId, descriptor);
    },
    [],
  );
  const handleRecoverTerminal = useCallback((tabId: string): Promise<void> => {
    return contextRef.current ? startRecovery(contextRef.current, tabId) : Promise.resolve();
  }, []);
  const beginCloseForTab = useCallback((tabId: string) => {
    return contextRef.current ? beginClose(contextRef.current, tabId) : false;
  }, []);
  const finishCloseForTab = useCallback((tabId: string, deleted: boolean) => {
    if (contextRef.current) finishClose(contextRef.current, tabId, deleted);
  }, []);

  return {
    terminalRecoveryByTab,
    handleTerminalStateChange,
    handleTerminalDescriptorReady,
    handleRecoverTerminal,
    beginClose: beginCloseForTab,
    finishClose: finishCloseForTab,
  };
}
