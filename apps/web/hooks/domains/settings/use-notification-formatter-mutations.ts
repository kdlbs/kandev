import {
  useCallback,
  useRef,
  type Dispatch,
  type MutableRefObject,
  type SetStateAction,
} from "react";
import type { NotificationFormatterBinding, NotificationFormatterChoice } from "@kdlbs/plugin-sdk";
import {
  selectNotificationFormatter,
  type NotificationFormatterSettings,
} from "@/lib/api/domains/notification-formatters-api";

export type NotificationFormatterState = {
  settings: NotificationFormatterSettings | null;
  selected: string | null;
  error: boolean;
  loading: boolean;
};

function sameChoice(
  current: NotificationFormatterChoice | undefined,
  captured: NotificationFormatterChoice,
) {
  return (
    current?.plugin_id === captured.plugin_id &&
    current?.installation_id === captured.installation_id &&
    current?.key === captured.key
  );
}

type MutationOptions = {
  providerId: string;
  choice: NotificationFormatterChoice | undefined;
  expectedRevision: string;
  generation: MutableRefObject<number>;
  scope: MutableRefObject<string | null>;
  selectionGeneration: MutableRefObject<number>;
  selectionVersion: number;
  setState: Dispatch<SetStateAction<NotificationFormatterState>>;
  readActiveBinding(): Promise<NotificationFormatterBinding | null>;
};

function useFormatterMutationPublication(options: MutationOptions) {
  const operation = useRef(0);
  const isCurrentSelection = () =>
    options.scope.current === options.providerId &&
    options.selectionGeneration.current === options.selectionVersion;
  const isCurrent = (sequence: number) => sequence === operation.current && isCurrentSelection();
  const begin = () => {
    options.generation.current++;
    return ++operation.current;
  };
  const assertCurrent = async (sequence: number) => {
    if (isCurrent(sequence)) return;
    if (options.scope.current === options.providerId) {
      try {
        await options.readActiveBinding();
      } catch {
        /* A stale acknowledgement is never authoritative. */
      }
    }
    // i18n-exempt: private cancellation error, never displayed as user copy.
    throw new DOMException("Formatter mutation superseded", "AbortError");
  };
  const publish = (binding: NotificationFormatterBinding, selected: string, sequence: number) => {
    if (!isCurrent(sequence)) return;
    options.generation.current++;
    options.setState((current) => ({
      ...current,
      loading: false,
      error: false,
      settings: current.settings ? { ...current.settings, binding } : null,
      selected: current.selected === selected ? null : current.selected,
    }));
  };
  return { begin, assertCurrent, isCurrent, isCurrentSelection, publish };
}

export function useNotificationFormatterMutations(options: MutationOptions) {
  const pendingClear = useRef<string | null>(null);
  const currentChoice = useRef(options.choice);
  currentChoice.current = options.choice;
  const publication = useFormatterMutationPublication(options);
  const activate = useCallback(
    async (prepared: { revision: string; digest: string }, expected: string) => {
      const { choice } = options;
      // i18n-exempt: private control-flow error, never displayed as user copy.
      if (!choice) throw new Error("Notification formatter selection unavailable");
      if (!publication.isCurrentSelection() || !sameChoice(currentChoice.current, choice)) {
        // i18n-exempt: private cancellation error, never displayed as user copy.
        throw new DOMException("Formatter editor removed", "AbortError");
      }
      const sequence = publication.begin();
      const result = await selectNotificationFormatter(options.providerId, expected, {
        plugin_id: choice.plugin_id,
        installation_id: choice.installation_id,
        formatter_key: choice.key,
        configuration_revision: prepared.revision,
        digest: prepared.digest,
      });
      await publication.assertCurrent(sequence);
      publication.publish(
        result.binding,
        JSON.stringify([choice.plugin_id, choice.installation_id, choice.key]),
        sequence,
      );
      return result.binding;
    },
    [options, publication],
  );
  const clear = useCallback(async () => {
    const sequence = publication.begin();
    let expected = options.expectedRevision;
    if (pendingClear.current === options.providerId) {
      const actual = await options.readActiveBinding();
      await publication.assertCurrent(sequence);
      pendingClear.current = null;
      if (!actual?.plugin_id) {
        if (actual) publication.publish(actual, "", sequence);
        return;
      }
      expected = actual.revision;
    }
    try {
      const result = await selectNotificationFormatter(options.providerId, expected, null);
      await publication.assertCurrent(sequence);
      publication.publish(result.binding, "", sequence);
    } catch (error) {
      if (!publication.isCurrent(sequence)) throw error;
      pendingClear.current = options.providerId;
      try {
        const actual = await options.readActiveBinding();
        await publication.assertCurrent(sequence);
        pendingClear.current = null;
        if (!actual?.plugin_id) {
          if (actual) publication.publish(actual, "", sequence);
          return;
        }
      } catch {
        /* Only authoritative readback permits another write. */
      }
      throw error;
    }
  }, [options, publication]);
  return { activate, clear };
}
