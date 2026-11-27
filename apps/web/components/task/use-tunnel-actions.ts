"use client";

import { useState, useCallback, useEffect, useMemo, useRef } from "react";
import { startTunnel, stopTunnel } from "@/lib/api/domains/port-api";
import { toast } from "@/lib/toast/sonner";
import { t } from "@/lib/i18n";

function useTunnelPending(sessionId: string) {
  const scope = useMemo(() => ({ sessionId }), [sessionId]);
  const currentScope = useRef<typeof scope | null>(scope);
  const [pending, setPending] = useState({ scope, ports: new Set<number>() });
  if (pending.scope !== scope) setPending({ scope, ports: new Set() });
  useEffect(() => {
    currentScope.current = scope;
    return () => {
      currentScope.current = null;
    };
  }, [scope]);
  const isCurrent = useCallback(() => currentScope.current === scope, [scope]);
  const setPendingTunnels = useCallback(
    (update: (previous: Set<number>) => Set<number>) => {
      setPending((previous) =>
        previous.scope === scope ? { scope, ports: update(previous.ports) } : previous,
      );
    },
    [scope],
  );
  return { pendingTunnels: pending.ports, setPendingTunnels, isCurrent };
}

export function useTunnelActions(
  sessionId: string,
  setActiveTunnels: (updater: (prev: Map<number, number>) => Map<number, number>) => void,
) {
  const { pendingTunnels, setPendingTunnels, isCurrent } = useTunnelPending(sessionId);

  const handleTunnelStart = useCallback(
    async (port: number, requestedPort?: number) => {
      setPendingTunnels((prev) => new Set(prev).add(port));
      try {
        const tunnelPort = await startTunnel(sessionId, port, requestedPort);
        if (!isCurrent()) return;
        setActiveTunnels((prev) => new Map(prev).set(port, tunnelPort));
        toast.success(t("task:tunnelStartedOnPort", { port: tunnelPort }));
      } catch (err) {
        if (!isCurrent()) return;
        toast.error(
          t("task:failedToStartTunnel", {
            error: err instanceof Error ? err.message : t("task:unknownError"),
          }),
        );
      } finally {
        setPendingTunnels((prev) => {
          const next = new Set(prev);
          next.delete(port);
          return next;
        });
      }
    },
    [sessionId, setActiveTunnels, setPendingTunnels, isCurrent],
  );

  const handleTunnelStop = useCallback(
    async (port: number) => {
      setPendingTunnels((prev) => new Set(prev).add(port));
      try {
        await stopTunnel(sessionId, port);
        if (!isCurrent()) return;
        setActiveTunnels((prev) => {
          const next = new Map(prev);
          next.delete(port);
          return next;
        });
        toast.success(t("task:tunnelStopped"));
      } catch (err) {
        if (!isCurrent()) return;
        toast.error(
          t("task:failedToStopTunnel", {
            error: err instanceof Error ? err.message : t("task:unknownError"),
          }),
        );
      } finally {
        setPendingTunnels((prev) => {
          const next = new Set(prev);
          next.delete(port);
          return next;
        });
      }
    },
    [sessionId, setActiveTunnels, setPendingTunnels, isCurrent],
  );

  return { pendingTunnels, handleTunnelStart, handleTunnelStop };
}
