import { useEffect, useState } from "react";
export const ORCHESTRATION_CHANGED = "kandev:orchestration-changed";
export const notifyOrchestrationChanged = () =>
  window.dispatchEvent(new Event(ORCHESTRATION_CHANGED));
export function useOrchestrationData<T>(load: () => Promise<T>) {
  const [data, setData] = useState<T>();
  const [error, setError] = useState<string>();
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    let active = true;
    setError(undefined);

    void load()
      .then((v) => {
        if (active) setData(v);
      })
      .catch((e) => {
        if (active) setError(String(e.message ?? e));
      });
    return () => {
      active = false;
    };
  }, [load, revision]);
  useEffect(() => {
    const changed = () => setRevision((v) => v + 1);
    window.addEventListener(ORCHESTRATION_CHANGED, changed);
    return () => window.removeEventListener(ORCHESTRATION_CHANGED, changed);
  }, []);
  return { data, error, refresh: () => setRevision((v) => v + 1) };
}
