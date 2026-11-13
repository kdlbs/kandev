"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getAutomation, getAutomationMCPOptions } from "@/lib/api/domains/automation-api";
import type { AutomationMCPOptions } from "@/lib/types/automation";

/**
 * Publish only the newest options response for the current workspace, profile, and repository
 * selection.
 */
export function useAutomationMCPOptions(
  workspaceId: string,
  profileId: string,
  repositoryIds: string[],
) {
  const key = JSON.stringify([workspaceId, profileId, repositoryIds]);
  const active = useRef({ key, sequence: 0, active: false });
  const [loaded, setLoaded] = useState<{
    key: string;
    options?: AutomationMCPOptions;
    error?: boolean;
  }>();
  const [pending, setPending] = useState<string>();
  const reload = useCallback(async () => {
    const owner = active.current;
    if (owner.key !== key || !owner.active) return;
    const sequence = ++owner.sequence;
    const current = () => owner.active && active.current === owner && owner.sequence === sequence;
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
    const owner = { key, sequence: 0, active: true };
    active.current = owner;
    void reload();
    return () => {
      owner.active = false;
    };
  }, [key, reload]);
  const matching = loaded?.key === key ? loaded : undefined;
  return {
    options: matching?.options,
    error: matching?.error ?? false,
    loading: pending === key || !matching,
    reload,
  };
}

/**
 * Reload saved selections only for the active editor owner and ignore superseded requests.
 */
export function useAutomationMCPSelectionReload(workspaceId: string, automationId: string | null) {
  const key = JSON.stringify([workspaceId, automationId]);
  const owner = useRef({ key, active: false, sequence: 0 });
  useEffect(() => {
    const captured = { key, active: true, sequence: 0 };
    owner.current = captured;
    return () => {
      captured.active = false;
    };
  }, [key]);
  return useCallback(async () => {
    const captured = owner.current;
    if (!automationId || captured.key !== key || !captured.active) return undefined;
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
