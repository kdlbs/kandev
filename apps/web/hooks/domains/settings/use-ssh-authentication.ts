"use client";
import { useEffect, useRef, useState } from "react";
import * as api from "@/lib/api/domains/ssh-connections-api";
import type { SSHRoute } from "@/lib/types/http-ssh-connections";

/** Fresh failures belong to the tested route revision, not every visible host. */
export function useSSHAuthentication(
  perform: (operation: () => Promise<SSHRoute>) => Promise<SSHRoute>,
  reload: () => Promise<void>,
) {
  const [failedTests, setFailedTests] = useState<Record<string, string>>({});
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  const test = (route: SSHRoute) =>
    perform(async () => {
      const id = route.destination.id;
      try {
        const result = await api.testManagedSSH(route);
        if (active.current)
          setFailedTests((previous) => {
            const next = { ...previous };
            delete next[id];
            return next;
          });
        return result;
      } catch (error) {
        if (active.current) setFailedTests((previous) => ({ ...previous, [id]: route.stamp }));
        // A rejected probe can persist invalidated readiness before returning an error.
        await reload().catch(() => undefined);
        throw error;
      }
    });
  return { test, failedTests };
}
