import { useCallback, useEffect, useState } from "react";

export type ComposerActivity = {
  draft?: boolean;
  busy?: boolean;
  required?: boolean;
  overlay?: boolean;
};

type DisclosureState = {
  scope: string;
  hovered: boolean;
  focused: boolean;
  revealed: boolean;
  manual: boolean;
  activity: Record<string, ComposerActivity>;
};

function initialState(scope: string): DisclosureState {
  return { scope, hovered: false, focused: false, revealed: false, manual: false, activity: {} };
}

function collectActivity(activity: DisclosureState["activity"]) {
  const values = Object.values(activity);
  return {
    forced: values.some((value) => value.busy || value.required),
    held: values.some((value) => value.draft || value.overlay || value.busy || value.required),
  };
}

function updateActivity(
  current: DisclosureState,
  owner: string,
  activity: ComposerActivity | null,
) {
  const keys = ["draft", "busy", "required", "overlay"] as const;
  if (keys.every((key) => Boolean(current.activity[owner]?.[key]) === Boolean(activity?.[key])))
    return current;
  const next = { ...current.activity };
  if (activity) next[owner] = activity;
  else delete next[owner];
  return {
    ...current,
    activity: next,
    revealed: current.revealed || (!current.manual && collectActivity(next).held),
  };
}

export function useComposerDisclosure({
  enabled,
  sessionId,
}: {
  enabled: boolean;
  sessionId: string | null;
}) {
  const scope = `${enabled}:${sessionId ?? ""}`;
  const [storedState, setState] = useState(() => initialState(scope));
  const state = storedState.scope === scope ? storedState : initialState(scope);
  if (storedState.scope !== scope) setState(state);

  const update = useCallback(
    (change: (current: DisclosureState) => DisclosureState) => {
      setState((current) => (current.scope === scope ? change(current) : current));
    },
    [scope],
  );
  const { forced, held } = collectActivity(state.activity);
  const expanded =
    !enabled || forced || (!state.manual && (state.revealed || state.focused || held));

  useEffect(() => {
    if (!enabled || !state.hovered) return;
    const timer = setTimeout(
      () => update((current) => ({ ...current, revealed: true, manual: false })),
      150,
    );
    return () => clearTimeout(timer);
  }, [enabled, state.hovered, update]);

  useEffect(() => {
    if (!enabled || state.hovered || state.focused || held || !state.revealed) return;
    const timer = setTimeout(() => update((current) => ({ ...current, revealed: false })), 300);
    return () => clearTimeout(timer);
  }, [enabled, state.hovered, state.focused, state.revealed, held, update]);

  const pointerEnter = useCallback(
    (pointerType: string) => {
      if (enabled && sessionId && pointerType !== "touch") {
        update((current) => ({ ...current, hovered: true }));
      }
    },
    [enabled, sessionId, update],
  );
  const pointerLeave = useCallback(
    (pointerType: string) => {
      if (pointerType !== "touch") update((current) => ({ ...current, hovered: false }));
    },
    [update],
  );
  const focus = useCallback(
    (suppressReveal = false) => {
      update((current) => ({
        ...current,
        focused: true,
        ...(!suppressReveal && { revealed: true, manual: false }),
      }));
    },
    [update],
  );
  const blur = useCallback(() => update((current) => ({ ...current, focused: false })), [update]);
  const reveal = useCallback(() => {
    update((current) => ({ ...current, revealed: true, manual: false }));
  }, [update]);
  const collapse = useCallback(() => {
    update((current) =>
      collectActivity(current.activity).forced
        ? current
        : { ...current, manual: true, revealed: false, focused: false, hovered: false },
    );
  }, [update]);
  const reportActivity = useCallback(
    (owner: string, activity: ComposerActivity | null) => {
      update((current) => updateActivity(current, owner, activity));
    },
    [update],
  );

  return {
    enabled,
    expanded,
    canCollapse: enabled && expanded && !forced,
    pointerEnter,
    pointerLeave,
    focus,
    blur,
    reveal,
    collapse,
    reportActivity,
  };
}
