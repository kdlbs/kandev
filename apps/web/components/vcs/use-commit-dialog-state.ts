import { useCallback, useLayoutEffect, useMemo, useRef, useState } from "react";

type Scope = { sessionId: string | null; environmentId: string | null };
type Draft = { message: string; body: string; stageAll: boolean; repo: string | undefined };
type Attempt = Draft & { scope: Scope; owner: object; revision: number };
type State = Draft & {
  scope: Scope;
  owner: object;
  active: boolean;
  revision: number;
  open: boolean;
  retained: boolean;
  pending: Attempt | null;
};

function fresh(scope: Scope): State {
  return {
    scope,
    owner: {},
    active: true,
    revision: 0,
    open: false,
    retained: false,
    pending: null,
    message: "",
    body: "",
    stageAll: false,
    repo: undefined,
  };
}

function useOwnedDraft(scope: Scope) {
  const [state, setState] = useState(() => fresh(scope));
  const current = useRef(state);
  useLayoutEffect(() => {
    const next = fresh(scope);
    current.current = next;
    setState(next);
    return () => {
      current.current = { ...current.current, active: false };
    };
  }, [scope]);

  const update = useCallback(
    (change: (value: State) => State) => {
      const value = current.current;
      if (!value.active || value.scope !== scope || value.owner !== state.owner) return;
      const next = change(value);
      current.current = next;
      setState(next);
    },
    [scope, state.owner],
  );
  return { state, update };
}

function admittedAttempt(draft: State, busy: boolean): Attempt | null {
  if (!draft.scope.sessionId || busy || draft.pending || !draft.message.trim()) return null;
  const { scope, owner, revision, message, body, stageAll, repo } = draft;
  return { scope, owner, revision, message, body, stageAll, repo };
}

export function useCommitDialogState(sessionId: string | null, environmentId: string | null) {
  const scope = useMemo(() => ({ sessionId, environmentId }), [sessionId, environmentId]);
  const { state, update } = useOwnedDraft(scope);

  const edit = useCallback(
    <K extends "message" | "body" | "stageAll">(key: K, value: Draft[K]) => {
      update((draft) =>
        draft[key] === value ? draft : { ...draft, [key]: value, revision: draft.revision + 1 },
      );
    },
    [update],
  );
  const setOpen = useCallback((open: boolean) => update((draft) => ({ ...draft, open })), [update]);
  const setMessage = useCallback((value: string) => edit("message", value), [edit]);
  const setBody = useCallback((value: string) => edit("body", value), [edit]);
  const setStageAll = useCallback((value: boolean) => edit("stageAll", value), [edit]);

  const openDialog = useCallback(
    (nextRepo?: string) => {
      // Direct onClick bindings may pass an event instead of a repository name.
      const repo = typeof nextRepo === "string" ? nextRepo : undefined;
      update((draft) =>
        draft.repo === repo && draft.retained
          ? { ...draft, open: true }
          : { ...fresh(scope), repo, open: true },
      );
    },
    [scope, update],
  );

  const begin = useCallback(
    (busy: boolean): Attempt | null => {
      let attempt: Attempt | null = null;
      update((draft) => {
        attempt = admittedAttempt(draft, busy);
        if (!attempt) return draft;
        return { ...draft, pending: attempt, retained: true };
      });
      return attempt;
    },
    [update],
  );

  const settle = useCallback(
    (attempt: Attempt, acknowledged: boolean) => {
      update((draft) => {
        if (draft.pending !== attempt) return draft;
        // Only the unchanged submitted draft is consumed by its acknowledgement.
        if (acknowledged && draft.revision === attempt.revision) return fresh(scope);
        return { ...draft, pending: null, retained: true };
      });
    },
    [scope, update],
  );

  const visible = state.scope === scope ? state : fresh(scope);
  return {
    open: visible.open,
    message: visible.message,
    body: visible.body,
    stageAll: visible.stageAll,
    repo: visible.repo,
    pending: visible.pending !== null,
    setOpen,
    setMessage,
    setBody,
    setStageAll,
    openDialog,
    begin,
    settle,
  };
}
