"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { ReactNode } from "react";
import { useToast } from "@/components/toast-provider";
import { updateTaskPortForwarding } from "@/lib/api/domains/kanban-api";
import type { Task } from "@/lib/types/http";
import { useTranslation } from "react-i18next";
import { canUsePortForwarding, isPortForwardingEnabled } from "./port-forwarding-visibility";

export type PortForwardingToggleOptions = {
  openDialogOnEnable?: boolean;
};

export type PortForwardingVisibility = {
  enabled: boolean;
  canToggle: boolean;
  isUpdating: boolean;
  dialogOpen: boolean;
  setDialogOpen: (open: boolean) => void;
  togglePortForwarding: (options?: PortForwardingToggleOptions) => Promise<void>;
};

type PortForwardingVisibilityProviderProps = {
  taskId?: string | null;
  metadata?: Task["metadata"];
  sessionId?: string | null;
  isAgentctlReady: boolean;
  isArchived?: boolean;
  children: ReactNode;
};

const PortForwardingVisibilityContext = createContext<PortForwardingVisibility | null>(null);

export function useOptionalPortForwardingVisibility(): PortForwardingVisibility | undefined {
  return useContext(PortForwardingVisibilityContext) ?? undefined;
}

export function usePortForwardingVisibility(): PortForwardingVisibility {
  const context = useOptionalPortForwardingVisibility();
  if (!context) {
    throw new Error(
      "usePortForwardingVisibility must be used within PortForwardingVisibilityProvider",
    );
  }
  return context;
}

export function PortForwardingVisibilityProvider({
  taskId,
  metadata,
  sessionId,
  isAgentctlReady,
  isArchived = false,
  children,
}: PortForwardingVisibilityProviderProps) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const persistedEnabled = isPortForwardingEnabled(metadata);
  const taskKey = taskId ?? "";
  const [enabled, setEnabled] = useState(persistedEnabled);
  const [isUpdating, setIsUpdating] = useState(false);
  const [dialogOpen, setDialogOpen] = useState(false);
  const lastTaskKeyRef = useRef(taskKey);
  const lastServerValueRef = useRef(persistedEnabled);
  const pendingServerValueRef = useRef<boolean | null>(null);
  const requestRef = useRef(0);

  const canToggle = !isArchived && canUsePortForwarding({ sessionId, isAgentctlReady });

  useEffect(() => {
    if (lastTaskKeyRef.current !== taskKey) {
      lastTaskKeyRef.current = taskKey;
      lastServerValueRef.current = persistedEnabled;
      pendingServerValueRef.current = null;
      requestRef.current += 1;
      setEnabled(persistedEnabled);
      setIsUpdating(false);
      setDialogOpen(false);
      return;
    }
    if (pendingServerValueRef.current !== null) {
      if (pendingServerValueRef.current === persistedEnabled) {
        pendingServerValueRef.current = null;
        lastServerValueRef.current = persistedEnabled;
      } else {
        return;
      }
    }
    if (!isUpdating && lastServerValueRef.current !== persistedEnabled) {
      lastServerValueRef.current = persistedEnabled;
      setEnabled(persistedEnabled);
    }
  }, [isUpdating, persistedEnabled, taskKey]);

  useEffect(() => {
    if (!canToggle) setDialogOpen(false);
  }, [canToggle]);

  const togglePortForwarding = useCallback(
    async (options?: PortForwardingToggleOptions) => {
      if (!taskId || !canToggle || isUpdating) return;
      const previousEnabled = enabled;
      const nextEnabled = !enabled;
      const requestId = requestRef.current + 1;
      requestRef.current = requestId;

      setEnabled(nextEnabled);
      setIsUpdating(true);
      if (!nextEnabled) setDialogOpen(false);

      try {
        await updateTaskPortForwarding(taskId, nextEnabled);
        if (requestRef.current !== requestId) return;
        pendingServerValueRef.current = nextEnabled;
        lastServerValueRef.current = nextEnabled;
        setEnabled(nextEnabled);
        if (nextEnabled && options?.openDialogOnEnable) setDialogOpen(true);
      } catch {
        if (requestRef.current !== requestId) return;
        setEnabled(previousEnabled);
        toast({
          variant: "error",
          description: t("task:portForwardingPreferenceUpdateFailed"),
        });
      } finally {
        if (requestRef.current === requestId) setIsUpdating(false);
      }
    },
    [canToggle, enabled, isUpdating, t, taskId, toast],
  );

  const value = useMemo<PortForwardingVisibility>(
    () => ({
      enabled,
      canToggle,
      isUpdating,
      dialogOpen,
      setDialogOpen,
      togglePortForwarding,
    }),
    [canToggle, dialogOpen, enabled, isUpdating, togglePortForwarding],
  );

  return (
    <PortForwardingVisibilityContext.Provider value={value}>
      {children}
    </PortForwardingVisibilityContext.Provider>
  );
}
