"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getAutomation, getAutomationMCPOptions } from "@/lib/api/domains/automation-api";
import type { AutomationMCPOptions } from "@/lib/types/automation";

export function useAutomationMCPOptions(
  workspaceId: string,
  profileId: string,
  repositoryIds: string[],
) {
  const key = JSON.stringify([workspaceId, profileId, repositoryIds]);
  const active = useRef({ key, sequence: 0 });
  if (active.current.key !== key) active.current = { key, sequence: 0 };
  const [loaded, setLoaded] = useState<{
    key: string;
    options?: AutomationMCPOptions;
    error?: boolean;
  }>();
  const [pending, setPending] = useState<string>();
  const reload = useCallback(async () => {
    const owner = active.current;
    const sequence = ++owner.sequence;
    const current = () => active.current === owner && owner.sequence === sequence;
    setPending(key);
    try {
      const options = await getAutomationMCPOptions(workspaceId, profileId, repositoryIds);
      if (current()) setLoaded({ key, options });
    } catch {
      if (current())
        setLoaded((previous) => ({
          key,
          options: previous?.key === key ? previous.options : undefined,
          error: true,
        }));
    } finally {
      if (current()) setPending(undefined);
    }
    // key captures the complete request identity.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  useEffect(() => {
    void reload();
    return () => {
      active.current.sequence++;
    };
  }, [reload]);
  const matching = loaded?.key === key ? loaded : undefined;
  return {
    options: matching?.options,
    error: matching?.error ?? false,
    loading: pending === key || !matching,
    reload,
  };
}

export function useAutomationMCPSelectionReload(workspaceId: string, automationId: string | null) {
  const key = JSON.stringify([workspaceId, automationId]);
  const owner = useRef({ key, active: true, sequence: 0 });
  if (owner.current.key !== key) owner.current = { key, active: true, sequence: 0 };
  useEffect(() => {
    const captured = owner.current;
    captured.active = true;
    return () => {
      captured.active = false;
    };
  }, [key]);
  const captured = owner.current;
  return useCallback(async () => {
    if (!automationId || captured !== owner.current || !captured.active) return undefined;
    const sequence = ++captured.sequence;
    const saved = await getAutomation(automationId);
    if (
      captured !== owner.current ||
      sequence !== captured.sequence ||
      !captured.active ||
      saved.id !== automationId ||
      saved.workspace_id !== workspaceId
    )
      return undefined;
    return saved;
  }, [key, automationId, workspaceId]);
}
