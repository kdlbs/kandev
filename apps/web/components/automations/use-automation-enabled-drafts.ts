"use client";

import { useCallback, useMemo, useState } from "react";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import type { Automation } from "@/lib/types/automation";

type EnabledOverrides = Record<string, boolean>;

type AutomationEnabledDraftsOptions = {
  automations: Automation[];
  enable: (id: string) => Promise<unknown>;
  disable: (id: string) => Promise<unknown>;
};

export function useAutomationEnabledDrafts({
  automations,
  enable,
  disable,
}: AutomationEnabledDraftsOptions) {
  const [overrides, setOverrides] = useState<EnabledOverrides>({});
  const dirtyEntries = useMemo(
    () =>
      Object.entries(overrides)
        .filter(([id, enabled]) => {
          const saved = automations.find((automation) => automation.id === id);
          return saved && saved.enabled !== enabled;
        })
        .sort(([left], [right]) => left.localeCompare(right)),
    [automations, overrides],
  );
  const revision = JSON.stringify(dirtyEntries);
  const dirtyIds = useMemo(() => new Set(dirtyEntries.map(([id]) => id)), [dirtyEntries]);

  const save = useCallback(async () => {
    let firstError: unknown;
    for (const [id, enabled] of dirtyEntries) {
      try {
        await (enabled ? enable(id) : disable(id));
        setOverrides((current) => {
          if (current[id] !== enabled) return current;
          const next = { ...current };
          delete next[id];
          return next;
        });
      } catch (error) {
        firstError ??= error;
      }
    }
    if (firstError) throw firstError;
  }, [dirtyEntries, disable, enable]);

  useSettingsSaveContributor({
    id: "automation-list-enabled",
    revision,
    isDirty: dirtyEntries.length > 0,
    save,
    discard: () => setOverrides({}),
  });

  return {
    automations: automations.map((automation) => ({
      ...automation,
      enabled: overrides[automation.id] ?? automation.enabled,
    })),
    dirtyIds,
    setEnabled: (id: string, enabled: boolean) =>
      setOverrides((current) => ({ ...current, [id]: enabled })),
  };
}
