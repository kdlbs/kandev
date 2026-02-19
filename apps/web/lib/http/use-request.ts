"use client";

import { useCallback, useEffect, useRef, useState } from "react";

export type RequestStatus = "idle" | "loading" | "success" | "error";

export type RequestState<T> = {
  status: RequestStatus;
  data: T | null;
  error: Error | null;
};

type UseRequestOptions = {
  successDuration?: number;
};

export function useRequest<TArgs extends unknown[], TData>(
  fn: (...args: TArgs) => Promise<TData>,
  options: UseRequestOptions = {},
) {
  const { successDuration = 1500 } = options;
  const [state, setState] = useState<RequestState<TData>>({
    status: "idle",
    data: null,
    error: null,
  });
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }
    };
  }, []);

  const run = useCallback(
    async (...args: TArgs) => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }
      setState({ status: "loading", data: null, error: null });
      try {
        const data = await fn(...args);
        setState({ status: "success", data, error: null });
        if (successDuration > 0) {
          timeoutRef.current = setTimeout(() => {
            setState((prev) => ({ ...prev, status: "idle" }));
          }, successDuration);
        }
        return data;
      } catch (error) {
        const normalized = error instanceof Error ? error : new Error("Request failed");
        setState({ status: "error", data: null, error: normalized });
        throw normalized;
      }
    },
    [fn, successDuration],
  );

  return {
    ...state,
    isLoading: state.status === "loading",
    run,
    reset: () => setState({ status: "idle", data: null, error: null }),
  };
}
