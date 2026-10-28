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
export function useSSHConnections() {
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
    async <T>(operation: () => Promise<T>, refresh = true): Promise<T> => {
      // Admission must precede React's next render, including same-tick submissions.
      if (pending.current) throw new Error();
      pending.current = true;
      setBusy(true);
      setError(null);
      try {
        const value = await operation();
        if (refresh) await reload();
        return value;
      } catch (e) {
        if (alive.current && sshManagerError(e) === "ssh_resource_in_use") {
          await reload().catch(() => undefined);
        }
        if (alive.current) setError(sshManagerError(e));
        throw e;
      } finally {
        pending.current = false;
        if (alive.current) setBusy(false);
      }
    },
    [reload],
  );
  const authentication = useSSHAuthentication(perform, reload);
  return {
    failedTests: authentication.failedTests,
    identities,
    routes,
    loaded,
    busy,
    error,
    reload: () => perform(() => reload(), false),
    createIdentity: (req: CreateSSHIdentity) => perform(() => api.createSSHIdentity(req)),
    renameIdentity: (identity: SSHIdentity, label: string) =>
      perform(() => api.renameSSHIdentity(identity, label)),
    deleteIdentity: (identity: SSHIdentity) => perform(() => api.deleteSSHIdentity(identity)),
    saveConnection: (req: SaveSSHConnection) => perform(() => api.saveManagedSSHConnection(req)),
    deleteConnection: (route: SSHRoute) => perform(() => api.deleteManagedSSHConnection(route)),
    observe: (route: SSHRoute) => perform(() => api.observeManagedSSH(route), false),
    trust: (route: SSHRoute, observation: SSHObservation) =>
      perform(() => api.trustManagedSSH(route, observation)),
    enroll: (route: SSHRoute, request: EnrollSSHKey) =>
      perform(() => api.enrollManagedSSH(route, request)),
    preflight: (route: SSHRoute) => perform(() => api.preflightManagedSSH(route), false),
    test: authentication.test,
  };
}
export type SSHManager = ReturnType<typeof useSSHConnections>;
