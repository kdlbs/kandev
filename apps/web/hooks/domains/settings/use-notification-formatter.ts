import { useCallback, useEffect, useRef, useState } from "react";
import type { NotificationFormatterBinding, NotificationFormatterChoice } from "@kdlbs/plugin-sdk";
import {
  getNotificationFormatter,
  previewNotificationFormatter,
} from "@/lib/api/domains/notification-formatters-api";
import {
  useNotificationFormatterMutations,
  type NotificationFormatterState,
} from "./use-notification-formatter-mutations";

export function formatterChoiceId(choice: NotificationFormatterChoice) {
  return JSON.stringify([choice.plugin_id, choice.installation_id, choice.key]);
}

export function formatterBindingId(binding: NotificationFormatterBinding | null | undefined) {
  return binding?.plugin_id
    ? JSON.stringify([binding.plugin_id, binding.installation_id, binding.formatter_key])
    : "";
}

export function useNotificationFormatter(providerId: string) {
  const [state, setState] = useState<NotificationFormatterState>({
    settings: null,
    selected: null,
    error: false,
    loading: true,
  });
  const generation = useRef(0);
  const scope = useRef<string | null>(providerId);
  const reload = useCallback(
    async (signal?: AbortSignal) => {
      const request = ++generation.current;
      setState((current) => ({ ...current, loading: true, error: false }));
      try {
        const settings = await getNotificationFormatter(providerId, { init: { signal } });
        if (!signal?.aborted && request === generation.current) {
          setState((current) => ({ ...current, settings, loading: false }));
        }
      } catch {
        if (!signal?.aborted && request === generation.current) {
          setState((current) => ({ ...current, error: true, loading: false }));
        }
      }
    },
    [providerId],
  );
  useEffect(() => {
    scope.current = providerId;
    const abort = new AbortController();
    void reload(abort.signal);
    return () => {
      scope.current = null;
      abort.abort();
      generation.current++;
    };
  }, [providerId, reload]);
  const selected = state.selected ?? formatterBindingId(state.settings?.binding);
  const choice = state.settings?.formatters.find((value) => formatterChoiceId(value) === selected);
  const readActiveBinding = useCallback(async () => {
    const request = ++generation.current;
    try {
      const settings = await getNotificationFormatter(providerId);
      // i18n-exempt: private cancellation error, never displayed as user copy.
      if (scope.current !== providerId || request !== generation.current)
        throw new DOMException("Formatter read superseded", "AbortError");
      setState((current) => ({ ...current, settings, loading: false, error: false }));
      return settings.binding;
    } catch (error) {
      if (scope.current === providerId && request === generation.current) {
        setState((current) => ({
          ...current,
          loading: false,
          error: current.settings ? current.error : true,
        }));
      }
      throw error;
    }
  }, [providerId]);
  const mutations = useNotificationFormatterMutations({
    providerId,
    choice,
    expectedRevision: state.settings?.binding?.revision ?? "",
    generation,
    scope,
    setState,
    readActiveBinding,
  });
  return {
    ...state,
    selected,
    choice,
    ...mutations,
    reload,
    readActiveBinding,
    preview: (draft: unknown, event: string, sample: string, signal?: AbortSignal) =>
      previewNotificationFormatter(
        providerId,
        {
          binding: {
            plugin_id: choice?.plugin_id,
            installation_id: choice?.installation_id,
            formatter_key: choice?.key,
          },
          event,
          sample,
          draft,
        },
        signal,
      ),
    select: (value: string) => setState((current) => ({ ...current, selected: value })),
    discard: () => setState((current) => ({ ...current, selected: null })),
  };
}
