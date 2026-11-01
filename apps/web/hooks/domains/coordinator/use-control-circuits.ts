import { useEffect, useState } from "react";
import { ApiError } from "@/lib/api/client";
import { getRoutingCircuits } from "@/lib/api/domains/routing-circuits-api";
import { isRoutingCircuits, type RoutingCircuit } from "@/lib/control-ui/routing-circuits";

type CircuitRead = {
  owner: object;
  identity: string;
  status: "ready" | "denied" | "unsupported" | "error";
  data?: RoutingCircuit[];
  receivedAt?: string;
  stale?: boolean;
};
type CircuitRevision = { owner: object; identity: string };

/** One read per authorized control snapshot, with no provider probe or polling. */
export function useControlRoutingCircuits(revision: CircuitRevision | undefined) {
  const [read, setRead] = useState<CircuitRead>();
  useEffect(() => {
    if (!revision) return;
    const current = revision;
    const controller = new AbortController();
    const fail = (status: "denied" | "unsupported" | "error") => {
      if (controller.signal.aborted) return;
      setRead((previous) => {
        const retained =
          status === "error" &&
          previous?.owner === current.owner &&
          previous.identity === current.identity &&
          previous.data !== undefined;
        return {
          owner: current.owner,
          identity: current.identity,
          status,
          ...(retained
            ? { data: previous.data, receivedAt: previous.receivedAt, stale: true }
            : {}),
        };
      });
    };
    void getRoutingCircuits({ cache: "no-store", init: { signal: controller.signal } })
      .then((response) => {
        if (controller.signal.aborted) return;
        if (isRoutingCircuits(response)) {
          setRead({
            owner: current.owner,
            identity: current.identity,
            status: "ready",
            data: response.circuits,
            receivedAt: new Date().toISOString(),
          });
        } else {
          fail("unsupported");
        }
      })
      .catch((error: unknown) => {
        fail(circuitReadFailure(error));
      });
    return () => controller.abort();
  }, [revision]);
  if (!revision || read?.owner !== revision.owner || read.identity !== revision.identity) {
    return { status: "loading" as const, data: undefined, receivedAt: undefined, stale: false };
  }
  return read;
}

function circuitReadFailure(error: unknown): "denied" | "unsupported" | "error" {
  if (error instanceof ApiError) {
    if (error.status === 401 || error.status === 403) return "denied";
    if (error.status === 404 || error.status === 501) return "unsupported";
  }
  return "error";
}
