import { useCallback, useEffect, useRef, useState } from "react";
import {
  getPreviewPolicy,
  setPreviewPolicy,
  type PreviewPolicy,
} from "@/lib/plugins/file-preview-client";
export function useFilePreviewPolicy(pluginId: string, workspaceId: string | null) {
  const generation = useRef<symbol>(Symbol());
  const [policy, setPolicy] = useState<PreviewPolicy | null>(null);
  const [failed, setFailed] = useState(false);
  const [saving, setSaving] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    generation.current = Symbol();
    setSaving(false);
    setPolicy(null);
    setFailed(false);
    if (workspaceId)
      void getPreviewPolicy({ pluginId, workspaceId }, controller.signal)
        .then((next) => {
          if (!controller.signal.aborted) setPolicy(next);
        })
        .catch(() => {
          if (!controller.signal.aborted) setFailed(true);
        });
    return () => {
      generation.current = Symbol();
      controller.abort();
    };
  }, [pluginId, workspaceId]);
  const update = useCallback(
    async (mode: PreviewPolicy["mode"]) => {
      if (!policy || !workspaceId) return;
      const request = generation.current;
      setSaving(true);
      setFailed(false);
      try {
        const next = await setPreviewPolicy({ pluginId, workspaceId }, mode, policy.revision);
        if (request === generation.current) setPolicy(next);
      } catch {
        if (request !== generation.current) return;
        setFailed(true);
        try {
          const current = await getPreviewPolicy({ pluginId, workspaceId });
          if (request === generation.current) setPolicy(current);
        } catch {
          if (request === generation.current) setPolicy(null);
        }
      } finally {
        if (request === generation.current) setSaving(false);
      }
    },
    [pluginId, workspaceId, policy],
  );
  return { policy, failed, saving, update };
}
