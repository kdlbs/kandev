"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/api/client";
import * as api from "@/lib/api/domains/ssh-connections-api";
import type {
  CreateSSHIdentity,
  EnrollSSHKey,
  SaveSSHConnection,
  SSHIdentity,
  SSHObservation,
  SSHRoute,
} from "@/lib/types/http-ssh-connections";
import { useSSHAuthentication } from "./use-ssh-authentication";
export function sshManagerError(error: unknown): string {
  if (
    error instanceof ApiError &&
    error.body &&
    typeof error.body === "object" &&
    "error" in error.body &&
    typeof error.body.error === "string"
  )
    return error.body.error;
  return "ssh_operation_failed";
}
export type SSHOperation = {
  kind: "observe" | "trust" | "test" | "enroll" | "preflight";
  connectionId: string;
  stamp: string;
  status: "pending" | "succeeded" | "failed";
  error?: string;
  hop?: "bastion" | "destination";
};
function operationHop(error: unknown): SSHOperation["hop"] {
  if (!(error instanceof ApiError) || !error.body || typeof error.body !== "object") return;
  const hop = "hop" in error.body ? error.body.hop : undefined;
  return hop === "bastion" || hop === "destination" ? hop : undefined;
}
export function useSSHConnections() {
  const [operation, setOperation] = useState<SSHOperation | null>(null);
  const [identities, setIdentities] = useState<SSHIdentity[]>([]);
  const [routes, setRoutes] = useState<SSHRoute[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const alive = useRef(true);
  const pending = useRef(false);
  const loadRevision = useRef(0);
  const reload = useCallback(async (signal?: AbortSignal) => {
    const revision = ++loadRevision.current;
    try {
      const [keys, connections] = await Promise.all([
        api.listSSHIdentities({ init: { signal } }),
        api.listManagedSSHConnections({ init: { signal } }),
      ]);
      if (alive.current && !signal?.aborted && revision === loadRevision.current) {
        setIdentities(keys);
        setRoutes(connections);
        setLoaded(true);
      }
    } catch (error) {
      if (alive.current && !signal?.aborted && revision === loadRevision.current) throw error;
    }
  }, []);
  useEffect(() => {
    alive.current = true;
    const controller = new AbortController();
    void reload(controller.signal).catch((e: unknown) => {
      if (!controller.signal.aborted) setError(sshManagerError(e));
    });
    return () => {
      alive.current = false;
      controller.abort();
    };
  }, [reload]);
  const perform = useCallback(
    async <T>(
      run: () => Promise<T>,
      refresh = true,
      context?: Omit<SSHOperation, "status" | "error" | "hop">,
    ): Promise<T> => {
      // Admission must precede React's next render, including same-tick submissions.
      if (pending.current) throw new Error();
      pending.current = true;
      setBusy(true);
      setError(null);
      setOperation(context ? { ...context, status: "pending" } : null);
      try {
        const value = await run();
        if (refresh) await reload();
        if (alive.current && context) setOperation({ ...context, status: "succeeded" });
        return value;
      } catch (e) {
        if (alive.current && sshManagerError(e) === "ssh_resource_in_use") {
          await reload().catch(() => undefined);
        }
        if (alive.current) {
          setError(sshManagerError(e));
          if (context)
            setOperation({
              ...context,
              status: "failed",
              error: sshManagerError(e),
              ...(operationHop(e) ? { hop: operationHop(e) } : {}),
            });
        }
        throw e;
      } finally {
        pending.current = false;
        if (alive.current) setBusy(false);
      }
    },
    [reload],
  );
  const authentication = useSSHAuthentication(
    (run, route) => perform(run, true, operationContext("test", route)),
    reload,
  );
  return {
    operation,
    failedTests: authentication.failedTests,
    identities,
    routes,
    loaded,
    busy,
    error,
    reload: () => perform(() => reload(), false),
    ...connectionCommands(perform),
    test: authentication.test,
  };
}
export type SSHManager = ReturnType<typeof useSSHConnections>;

type SSHPerform = <T>(
  run: () => Promise<T>,
  refresh?: boolean,
  context?: Omit<SSHOperation, "status" | "error" | "hop">,
) => Promise<T>;
function operationContext(kind: SSHOperation["kind"], route: SSHRoute) {
  return { kind, connectionId: route.destination.id, stamp: route.stamp };
}
function connectionCommands(perform: SSHPerform) {
  return {
    createIdentity: (req: CreateSSHIdentity) => perform(() => api.createSSHIdentity(req)),
    renameIdentity: (identity: SSHIdentity, label: string) =>
      perform(() => api.renameSSHIdentity(identity, label)),
    deleteIdentity: (identity: SSHIdentity) => perform(() => api.deleteSSHIdentity(identity)),
    saveConnection: (req: SaveSSHConnection) => perform(() => api.saveManagedSSHConnection(req)),
    deleteConnection: (route: SSHRoute) => perform(() => api.deleteManagedSSHConnection(route)),
    observe: (route: SSHRoute) =>
      perform(() => api.observeManagedSSH(route), false, operationContext("observe", route)),
    trust: (route: SSHRoute, observation: SSHObservation) =>
      perform(
        () => api.trustManagedSSH(route, observation),
        true,
        operationContext("trust", route),
      ),
    enroll: (route: SSHRoute, request: EnrollSSHKey) =>
      perform(() => api.enrollManagedSSH(route, request), true, operationContext("enroll", route)),
    preflight: (route: SSHRoute) =>
      perform(() => api.preflightManagedSSH(route), false, operationContext("preflight", route)),
  };
}
