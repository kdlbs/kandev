"use client";

import { useCallback, useMemo, useState } from "react";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";

type EnabledItem = { id: string; enabled: boolean };

type WatcherEnabledDraftOptions<T extends EnabledItem> = {
  id: string;
  items: T[];
  saveEnabled: (item: T, enabled: boolean) => Promise<void>;
};

export function useWatcherEnabledDrafts<T extends EnabledItem>({
  id,
  items,
  saveEnabled,
}: WatcherEnabledDraftOptions<T>) {
  const [drafts, setDrafts] = useState<Record<string, boolean>>({});
  const changes = useMemo(
    () =>
      items
        .filter((item) => drafts[item.id] !== undefined && drafts[item.id] !== item.enabled)
        .map((item) => ({ item, enabled: drafts[item.id] }))
        .sort((left, right) => left.item.id.localeCompare(right.item.id)),
    [drafts, items],
  );
  const draftItems = useMemo(
    () => items.map((item) => ({ ...item, enabled: drafts[item.id] ?? item.enabled })),
    [drafts, items],
  );
  const dirtyIds = useMemo(() => new Set(changes.map(({ item }) => item.id)), [changes]);

  const toggleEnabled = useCallback((item: T) => {
    setDrafts((current) => ({
      ...current,
      [item.id]: !(current[item.id] ?? item.enabled),
    }));
  }, []);
  const discard = useCallback(() => setDrafts({}), []);
  const save = useCallback(async () => {
    const results = await Promise.allSettled(
      changes.map(async ({ item, enabled }) => {
        await saveEnabled(item, enabled);
        setDrafts((current) => {
          if (current[item.id] !== enabled) return current;
          const next = { ...current };
          delete next[item.id];
          return next;
        });
      }),
    );
    if (results.some((result) => result.status === "rejected")) {
      throw new Error("Failed to update one or more watchers");
    }
  }, [changes, saveEnabled]);

  useSettingsSaveContributor({
    id,
    revision: JSON.stringify(changes.map(({ item, enabled }) => [item.id, enabled])),
    isDirty: changes.length > 0,
    save,
    discard,
  });

  return { items: draftItems, dirtyIds, toggleEnabled };
}
