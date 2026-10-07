import { useLayoutEffect, useState } from "react";

function createLifetime(key: unknown, enabled: boolean) {
  return {
    key,
    enabled,
    identity: Symbol(),
    active: false,
    retired: false,
    loading: Symbol(),
    saving: Symbol(),
    syncing: Symbol(),
  };
}

export type WorkflowSyncLifetime = ReturnType<typeof createLifetime>;

// Callbacks capture this activation. A retired activation is never reused,
// including when StrictMode replays setup without changing the workspace.
export function useWorkflowSyncLifetime(key: unknown, enabled = true) {
  const [lifetime, setLifetime] = useState(() => createLifetime(key, enabled));
  const rendered =
    lifetime.key === key && lifetime.enabled === enabled ? lifetime : createLifetime(key, enabled);
  if (rendered !== lifetime) setLifetime(rendered);

  useLayoutEffect(() => {
    if (rendered.retired) {
      setLifetime(createLifetime(rendered.key, rendered.enabled));
      return;
    }
    rendered.active = rendered.enabled;
    return () => {
      rendered.active = false;
      rendered.retired = true;
    };
  }, [rendered]);

  return rendered;
}
