"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { fetchSandboxBinding, updateSandboxBinding } from "@/lib/api/domains/sandbox-api";
import type { SandboxBinding, SandboxBindingScope } from "@/lib/types/sandbox";
import { sandboxErrorMessageKey } from "@/lib/sandbox-messages";

type State = {
  key: string;
  saved: SandboxBinding | null;
  draft: SandboxBinding | null;
  loading: boolean;
  saving: boolean;
  error: boolean;
  errorMessage: ReturnType<typeof sandboxErrorMessageKey> | null;
};
const UNAVAILABLE = Symbol();

function failed(current: State, reason: unknown): State {
  return {
    ...current,
    loading: false,
    saving: false,
    error: true,
    errorMessage: sandboxErrorMessageKey(reason),
  };
}

function initial(key: string): State {
  return {
    key,
    saved: null,
    draft: null,
    loading: true,
    saving: false,
    error: false,
    errorMessage: null,
  };
}

export function useSandboxBinding(
  scope: SandboxBindingScope,
  targetId: string,
  { enabled = true, identity = "" }: { enabled?: boolean; identity?: string } = {},
) {
  const key = JSON.stringify([scope, targetId, enabled, identity]);
  const [state, setState] = useState<State>(() => initial(key));
  const generation = useRef(0);
  const savingGeneration = useRef<number | null>(null);
  useEffect(() => {
    const epoch = ++generation.current;
    savingGeneration.current = null;
    const controller = new AbortController();
    setState(initial(key));
    if (!enabled) return;
    void fetchSandboxBinding(scope, targetId, { init: { signal: controller.signal } }).then(
      (saved) => {
        if (generation.current === epoch)
          setState({
            key,
            saved,
            draft: saved,
            loading: false,
            saving: false,
            error: false,
            errorMessage: null,
          });
      },
      (reason) => {
        if (generation.current === epoch) setState(failed(initial(key), reason));
      },
    );
    return () => {
      generation.current++;
      controller.abort();
    };
  }, [scope, targetId, key, enabled]);

  const update = useCallback(
    (patch: Partial<Pick<SandboxBinding, "revision_id" | "executor">>) => {
      setState((current) =>
        current.key === key && current.draft
          ? { ...current, draft: { ...current.draft, ...patch }, error: false, errorMessage: null }
          : current,
      );
    },
    [key],
  );
  const discard = useCallback(() => {
    setState((current) =>
      current.key === key
        ? { ...current, draft: current.saved, error: false, errorMessage: null }
        : current,
    );
  }, [key]);
  const save = useCallback(async () => {
    if (
      state.key !== key ||
      !state.draft ||
      state.loading ||
      state.saving ||
      savingGeneration.current !== null
    )
      throw UNAVAILABLE;
    const submitted = state.draft;
    const epoch = generation.current;
    savingGeneration.current = epoch;
    setState((current) => ({ ...current, saving: true, error: false, errorMessage: null }));
    try {
      const saved = await updateSandboxBinding(submitted);
      if (generation.current !== epoch) throw UNAVAILABLE;
      setState((current) => ({
        ...current,
        saved,
        saving: false,
        draft:
          JSON.stringify(current.draft) === JSON.stringify(submitted)
            ? saved
            : { ...current.draft!, version: saved.version },
      }));
    } catch (cause) {
      if (generation.current === epoch) setState((current) => failed(current, cause));
      throw cause;
    } finally {
      if (savingGeneration.current === epoch) savingGeneration.current = null;
    }
  }, [key, state]);
  const current = state.key === key && enabled ? state : { ...initial(key), loading: enabled };
  return {
    ...current,
    dirty: JSON.stringify(current.saved) !== JSON.stringify(current.draft),
    update,
    discard,
    save,
  };
}
