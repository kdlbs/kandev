"use client";

import { useCallback, useState } from "react";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";

type DraftedIntegrationEnabledOptions = {
  id: string;
  enabled: boolean;
  persist: (enabled: boolean) => Promise<void> | void;
};

export function useDraftedIntegrationEnabled({
  id,
  enabled,
  persist,
}: DraftedIntegrationEnabledOptions) {
  const [baseline, setBaseline] = useState(enabled);
  const [draft, setDraft] = useState(enabled);

  if (enabled !== baseline && draft === baseline) {
    setBaseline(enabled);
    setDraft(enabled);
  }

  const save = useCallback(async () => {
    const submitted = draft;
    await persist(submitted);
    setBaseline(submitted);
  }, [draft, persist]);
  const discard = useCallback(() => setDraft(baseline), [baseline]);

  useSettingsSaveContributor({
    id,
    revision: Number(draft),
    isDirty: draft !== baseline,
    save,
    discard,
  });

  return { enabled: draft, setEnabled: setDraft, isDirty: draft !== baseline };
}
