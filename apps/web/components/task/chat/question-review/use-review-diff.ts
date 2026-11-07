import { useEffect, useRef, useState } from "react";
import { createPixelDiffWorker } from "@/lib/question-review/diff-worker";
import type { DiffRequest, DiffResponse } from "@/lib/question-review/diff.worker";

export type ReviewDiffResult = {
  width: number;
  height: number;
  changed: number;
  total: number;
  mask: Uint8ClampedArray;
};

export type ReviewDiffState =
  | { status: "idle" }
  | { status: "computing" }
  | { status: "ready"; result: ReviewDiffResult }
  | { status: "unavailable" };

const TOLERANCE_DEBOUNCE_MS = 150;
const blobIds = new WeakMap<Blob, number>();
let nextBlobId = 0;

function blobId(blob: Blob): number {
  let id = blobIds.get(blob);
  if (id === undefined) {
    id = ++nextBlobId;
    blobIds.set(blob, id);
  }
  return id;
}

/**
 * Computes the pixel difference of two loaded images in a worker. Results that
 * arrive after the inputs changed are discarded. The worker ends with the stage.
 */
export function useReviewDiff(
  enabled: boolean,
  a: Blob | undefined,
  b: Blob | undefined,
  tolerance: number,
): ReviewDiffState {
  const workerRef = useRef<Worker | null | undefined>(undefined);
  const requestRef = useRef(0);
  const [state, setState] = useState<ReviewDiffState>({ status: "idle" });

  useEffect(
    () => () => {
      workerRef.current?.terminate();
      workerRef.current = undefined;
    },
    [],
  );

  useEffect(() => {
    if (!enabled || !a || !b) return;
    if (workerRef.current === undefined) workerRef.current = createPixelDiffWorker();
    const worker = workerRef.current;
    const id = ++requestRef.current;
    if (!worker) {
      setState({ status: "unavailable" });
      return;
    }
    setState({ status: "computing" });
    const onMessage = (event: MessageEvent<DiffResponse>) => {
      const response = event.data;
      if (response.id !== requestRef.current) return;
      if (!response.ok) {
        setState({ status: "unavailable" });
        return;
      }
      const { width, height, changed, total } = response;
      setState({
        status: "ready",
        result: { width, height, changed, total, mask: new Uint8ClampedArray(response.mask) },
      });
    };
    const onError = () => {
      if (id === requestRef.current) setState({ status: "unavailable" });
    };
    worker.addEventListener("message", onMessage);
    worker.addEventListener("error", onError);
    const timer = setTimeout(() => {
      const request: DiffRequest = { id, pairKey: `${blobId(a)}:${blobId(b)}`, a, b, tolerance };
      worker.postMessage(request);
    }, TOLERANCE_DEBOUNCE_MS);
    return () => {
      clearTimeout(timer);
      worker.removeEventListener("message", onMessage);
      worker.removeEventListener("error", onError);
    };
  }, [enabled, a, b, tolerance]);

  return enabled && a && b ? state : { status: "idle" };
}
