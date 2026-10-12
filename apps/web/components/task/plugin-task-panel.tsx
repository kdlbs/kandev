"use client";

import { useEffect, useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { PluginErrorBoundary } from "@/components/plugins/plugin-error-boundary";
import {
  PluginConversationScopeProvider,
  pluginConversationApi,
} from "@/lib/plugins/conversation-host";
import type { PluginTaskPanelRegistration } from "@/lib/plugins/registry-registration-types";
import { pluginRegistry, usePluginRegistry } from "@/lib/plugins/registry";
import type {
  PluginOpenMessageResult,
  PluginPresentation,
  PluginTaskPanelContext,
  PluginSessionKind,
} from "@/lib/plugins/types";
import { useDockviewStore } from "@/lib/state/dockview-store";

export interface PluginTaskPanelContainerProps {
  pluginId: string;
  panelKey: string;
  /** Full dockview/mobile panel id, e.g. `plugin:<pluginId>:<panelKey>`. */
  panelId: string;
  presentation: PluginPresentation;
  onOpenMessage?: (messageId: string) => PluginOpenMessageResult;
}

function PluginTaskPanelUnavailable() {
  const { t } = useTranslation();
  return (
    <div className="flex h-full items-center justify-center p-8 text-center text-sm text-muted-foreground">
      {t("common:pluginPanelUnavailable")}
    </div>
  );
}

function PluginTaskPanelFailed() {
  const { t } = useTranslation();
  return (
    <div className="flex h-full items-center justify-center p-8 text-center text-sm text-muted-foreground">
      {t("common:pluginPanelFailedToLoad")}
    </div>
  );
}

export function registrationIsVisible(
  registration: PluginTaskPanelRegistration,
  context: PluginTaskPanelContext,
): boolean {
  if (!registration.visible) return true;
  try {
    return registration.visible(context);
  } catch (error) {
    console.error(
      `[plugins] visibility predicate for "${registration.pluginId}:${registration.id}" threw`,
      error,
    );
    return false;
  }
}

type RememberedSession = { taskId: string | null; sessionId: string };

function useRememberedPanelSessionId(
  taskId: string | null,
  activeSessionId: string | null,
): string | null {
  const rememberedSession = useRef<RememberedSession | null>(null);
  if (activeSessionId) {
    rememberedSession.current = { taskId, sessionId: activeSessionId };
  }
  return (
    activeSessionId ??
    (rememberedSession.current?.taskId === taskId ? rememberedSession.current.sessionId : null)
  );
}

/** Resolves and contains one plugin-contributed task panel. */
export function PluginTaskPanel({
  pluginId,
  panelKey,
  panelId,
  presentation,
  onOpenMessage,
}: PluginTaskPanelContainerProps) {
  usePluginRegistry();
  const { t } = useTranslation();
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  // Keep the deleted session identity until the host unmounts this panel.
  const sessionId = useRememberedPanelSessionId(taskId, activeSessionId);
  const session = useAppStore((state) =>
    sessionId ? state.taskSessions?.items?.[sessionId] : undefined,
  );
  const registration = pluginRegistry.getTaskPanel(pluginId, panelKey);
  const generation = pluginRegistry.getPluginLifecycle(pluginId)?.generation ?? 0;
  let sessionKind: PluginSessionKind = null;
  if (sessionId) {
    sessionKind = session?.is_passthrough ? "passthrough" : "managed";
  }
  const context: PluginTaskPanelContext = {
    taskId: taskId ?? "",
    sessionId,
    sessionKind,
    presentation,
  };
  const registrationVisible = registration ? registrationIsVisible(registration, context) : false;

  const lease = useMemo(
    () => ({ active: true }),
    [generation, panelId, panelKey, pluginId, presentation, registrationVisible, sessionId, taskId],
  );
  useEffect(
    () => () => {
      lease.active = false;
    },
    [lease],
  );
  const conversation = useMemo(
    () => ({
      history: pluginConversationApi,
      openMessage(messageId: string): PluginOpenMessageResult {
        if (!lease.active || !sessionId || messageId.trim() === "") {
          return { status: "unavailable" };
        }
        if (presentation === "mobile") {
          return onOpenMessage?.(messageId) ?? { status: "unavailable" };
        }
        const queued = useDockviewStore
          .getState()
          .scrollTranscriptToMessage(sessionId, messageId, session?.name || t("task:chat"));
        return { status: queued ? "accepted" : "unavailable" };
      },
    }),
    [lease, onOpenMessage, presentation, session?.name, sessionId, t],
  );

  if (!registration || !taskId || !registrationVisible) {
    return <PluginTaskPanelUnavailable />;
  }

  return (
    <PluginErrorBoundary context={`task panel "${panelId}"`} fallback={<PluginTaskPanelFailed />}>
      <PluginConversationScopeProvider
        pluginId={pluginId}
        taskId={taskId}
        sessionId={sessionId}
        generation={generation}
        presentation={presentation}
      >
        <registration.Component
          panelId={panelId}
          taskId={taskId}
          sessionId={sessionId}
          sessionKind={sessionKind}
          presentation={presentation}
          conversation={conversation}
        />
      </PluginConversationScopeProvider>
    </PluginErrorBoundary>
  );
}
