import { useCallback, useEffect, useState } from "react";
export const ORCHESTRATION_CHANGED = "kandev:orchestration-changed";
export const notifyOrchestrationChanged = () =>
  window.dispatchEvent(new Event(ORCHESTRATION_CHANGED));
export function useOrchestrationData<T>(load: () => Promise<T>, scopeKey?: string) {
  const [snapshot, setSnapshot] = useState<{ load: typeof load; scopeKey?: string; data: T }>();
  const [error, setError] = useState<{ load: typeof load; scopeKey?: string; message: string }>();
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    let active = true;
    setError(undefined);

    void load()
      .then((v) => {
        if (active) setSnapshot({ load, scopeKey, data: v });
      })
      .catch((e) => {
        if (active) setError({ load, scopeKey, message: String(e.message ?? e) });
      });
    return () => {
      active = false;
    };
  }, [load, revision, scopeKey]);
  useEffect(() => {
    const changed = () => setRevision((v) => v + 1);
    window.addEventListener(ORCHESTRATION_CHANGED, changed);
    return () => window.removeEventListener(ORCHESTRATION_CHANGED, changed);
  }, []);
  const refresh = useCallback(() => setRevision((value) => value + 1), []);
  return {
    data: snapshot?.load === load && snapshot.scopeKey === scopeKey ? snapshot.data : undefined,
    error: error?.load === load && error.scopeKey === scopeKey ? error.message : undefined,
    refresh,
  };
}
