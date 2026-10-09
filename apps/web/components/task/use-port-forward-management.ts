import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { listPorts, listTunnels, type ListeningPort } from "@/lib/api/domains/port-api";
import { toast } from "@/lib/toast/sonner";
import { useTunnelActions } from "./use-tunnel-actions";

type Scope = { sessionId: string | null };

function initialState(scope: Scope) {
  return {
    scope,
    activeTunnels: new Map<number, number>(),
    mutatedPorts: new Set<number>(),
    detectedPorts: [] as ListeningPort[],
    manualPorts: [] as number[],
    manualValue: "",
    loading: false,
    loaded: false,
  };
}

type State = ReturnType<typeof initialState>;

function retainTargets(previous: number[], tunnels: Map<number, number>) {
  return [...new Set([...previous, ...tunnels.keys()])];
}

function updateTunnels(state: State, activeTunnels: Map<number, number>) {
  const mutatedPorts = new Set(state.mutatedPorts);
  for (const port of new Set([...state.activeTunnels.keys(), ...activeTunnels.keys()])) {
    if (state.activeTunnels.get(port) !== activeTunnels.get(port)) mutatedPorts.add(port);
  }
  return {
    ...state,
    activeTunnels,
    mutatedPorts,
    manualPorts: retainTargets(state.manualPorts, activeTunnels),
  };
}

export function usePortForwardManagement(sessionId: string | null, active: boolean) {
  const { t } = useTranslation();
  const scope = useMemo(() => ({ sessionId }), [sessionId]);
  const [state, setState] = useState(() => initialState(scope));
  if (state.scope !== scope) setState(initialState(scope));

  const setActiveTunnels = useCallback(
    (updater: (previous: Map<number, number>) => Map<number, number>) => {
      setState((previous) =>
        previous.scope === scope
          ? updateTunnels(previous, updater(previous.activeTunnels))
          : previous,
      );
    },
    [scope],
  );
  const actions = useTunnelActions(sessionId ?? "", setActiveTunnels);

  useEffect(() => {
    if (!active || !sessionId) return;
    let cancelled = false;
    void listTunnels(sessionId).then((tunnels) => {
      if (cancelled) return;
      setState((previous) => {
        if (previous.scope !== scope) return previous;
        const hydrated = new Map(tunnels.map((tunnel) => [tunnel.port, tunnel.tunnel_port]));
        for (const port of previous.mutatedPorts) {
          const tunnelPort = previous.activeTunnels.get(port);
          if (tunnelPort === undefined) hydrated.delete(port);
          else hydrated.set(port, tunnelPort);
        }
        return {
          ...previous,
          activeTunnels: hydrated,
          manualPorts: retainTargets(previous.manualPorts, hydrated),
        };
      });
    });
    return () => {
      cancelled = true;
    };
  }, [active, scope, sessionId]);

  const refresh = useCallback(async () => {
    if (!sessionId) return;
    setState((previous) => (previous.scope === scope ? { ...previous, loading: true } : previous));
    try {
      const detectedPorts = await listPorts(sessionId);
      setState((previous) =>
        previous.scope === scope ? { ...previous, detectedPorts, loaded: true } : previous,
      );
    } finally {
      setState((previous) =>
        previous.scope === scope ? { ...previous, loading: false } : previous,
      );
    }
  }, [scope, sessionId]);

  const setManualValue = useCallback(
    (manualValue: string) => {
      setState((previous) => (previous.scope === scope ? { ...previous, manualValue } : previous));
    },
    [scope],
  );

  const addManualPort = useCallback(() => {
    const port = parseInt(state.manualValue, 10);
    if (isNaN(port) || port < 1 || port > 65535) {
      toast.error(t("task:enterAValidPort1655352"));
      return;
    }
    if (state.manualPorts.includes(port)) {
      toast.error(t("task:portAlreadyAdded"));
      return;
    }
    setState((previous) =>
      previous.scope === scope
        ? { ...previous, manualPorts: [...previous.manualPorts, port], manualValue: "" }
        : previous,
    );
  }, [scope, state.manualPorts, state.manualValue, t]);

  return { ...state, ...actions, refresh, setManualValue, addManualPort };
}

export type PortForwardManagement = ReturnType<typeof usePortForwardManagement>;
