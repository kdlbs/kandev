"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  getDependencies,
  mergeErrorCode,
  putDependencies,
  type DepRef,
} from "@/lib/api/domains/coordinator-merge-api";
import { addRef, depErrorKey, removeRef, sameRefs } from "@/lib/coordinators/merge-dependencies";

export type DependenciesStatus = "idle" | "loading" | "ready" | "error";

/**
 * A card's predecessor set: read when `enabled`, edited locally, and saved whole with the
 * revision it was read at. A stale revision reloads the set and says so.
 */
export function useTaskDependencies(
  workspaceId: string,
  coordinatorId: string,
  taskId: string,
  enabled: boolean,
) {
  const [stored, setStored] = useState<{ refs: DepRef[]; revision: number } | null>(null);
  const [refs, setRefs] = useState<DepRef[]>([]);
  const [status, setStatus] = useState<DependenciesStatus>("idle");
  const [errorKey, setErrorKey] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const lockRef = useRef(false);
  const sequenceRef = useRef(0);

  const load = useCallback(() => {
    const sequence = ++sequenceRef.current;
    setStatus("loading");
    getDependencies(workspaceId, coordinatorId, taskId)
      .then((set) => {
        if (sequence !== sequenceRef.current) return;
        setStored(set);
        setRefs(set.refs);
        setStatus("ready");
      })
      .catch(() => {
        if (sequence !== sequenceRef.current) return;
        setStatus("error");
      });
  }, [workspaceId, coordinatorId, taskId]);

  useEffect(() => {
    if (!enabled) return;
    load();
    return () => {
      sequenceRef.current += 1;
    };
  }, [enabled, load]);

  const add = useCallback(
    (input: string): boolean => {
      const result = addRef(refs, input, taskId);
      if (!result.ok) {
        const keys = {
          empty: "coordinator:mergeDepInvalid",
          duplicate: "coordinator:mergeDepDuplicate",
          self: "coordinator:mergeDepSelf",
        } as const;
        setErrorKey(keys[result.reason]);
        return false;
      }
      setErrorKey(null);
      setSaved(false);
      setRefs(result.refs);
      return true;
    },
    [refs, taskId],
  );

  const remove = useCallback((ref: DepRef) => {
    setErrorKey(null);
    setSaved(false);
    setRefs((current) => removeRef(current, ref));
  }, []);

  const save = useCallback(async () => {
    if (!stored || lockRef.current) return;
    lockRef.current = true;
    setSaving(true);
    setErrorKey(null);
    try {
      const set = await putDependencies(workspaceId, coordinatorId, taskId, stored.revision, refs);
      setStored(set);
      setRefs(set.refs);
      setSaved(true);
    } catch (error) {
      const code = mergeErrorCode(error);
      setErrorKey(depErrorKey(code));
      if (code === "revision_conflict" || code === "revision_required") load();
    } finally {
      lockRef.current = false;
      setSaving(false);
    }
  }, [stored, workspaceId, coordinatorId, taskId, refs, load]);

  return {
    refs,
    status,
    errorKey,
    saving,
    saved,
    dirty: !!stored && !sameRefs(stored.refs, refs),
    add,
    remove,
    save,
    retry: load,
  };
}
