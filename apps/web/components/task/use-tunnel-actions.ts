"use client";

import { useState, useCallback } from "react";
import { startTunnel, stopTunnel } from "@/lib/api/domains/port-api";
import { toast } from "sonner";

export function useTunnelActions(
  sessionId: string,
  setActiveTunnels: (updater: (prev: Map<number, number>) => Map<number, number>) => void,
) {
  const [pendingTunnels, setPendingTunnels] = useState<Set<number>>(new Set());

  const handleTunnelStart = useCallback(
    async (port: number, requestedPort?: number) => {
      setPendingTunnels((prev) => new Set(prev).add(port));
      try {
        const tunnelPort = await startTunnel(sessionId, port, requestedPort);
        setActiveTunnels((prev) => new Map(prev).set(port, tunnelPort));
        toast.success(`Tunnel started on port ${tunnelPort}`);
      } catch (err) {
        toast.error(
          `Failed to start tunnel: ${err instanceof Error ? err.message : "unknown error"}`,
        );
      } finally {
        setPendingTunnels((prev) => {
          const next = new Set(prev);
          next.delete(port);
          return next;
        });
      }
    },
    [sessionId, setActiveTunnels],
  );

  const handleTunnelStop = useCallback(
    async (port: number) => {
      setPendingTunnels((prev) => new Set(prev).add(port));
      try {
        await stopTunnel(sessionId, port);
        setActiveTunnels((prev) => {
          const next = new Map(prev);
          next.delete(port);
          return next;
        });
        toast.success("Tunnel stopped");
      } catch (err) {
        toast.error(
          `Failed to stop tunnel: ${err instanceof Error ? err.message : "unknown error"}`,
        );
      } finally {
        setPendingTunnels((prev) => {
          const next = new Set(prev);
          next.delete(port);
          return next;
        });
      }
    },
    [sessionId, setActiveTunnels],
  );

  return { pendingTunnels, handleTunnelStart, handleTunnelStop };
}
