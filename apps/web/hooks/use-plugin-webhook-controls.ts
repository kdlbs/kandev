import { useEffect, useRef, useState } from "react";
import { getBackendConfig } from "@/lib/config";
import {
  managePluginWebhook,
  listPluginWebhookReceipts,
  type PluginWebhookBinding,
  type PluginWebhookReceipt,
} from "@/lib/api/domains/automation-api";
import type { AutomationTrigger } from "@/lib/types/automation";

export function useWebhookControls(trigger: AutomationTrigger, dirty: boolean) {
  const [binding, setBinding] = useState<PluginWebhookBinding | null>(null);
  const [receipts, setReceipts] = useState<PluginWebhookReceipt[]>([]);
  const [busy, setBusy] = useState(Boolean(trigger.automation_id));
  const [error, setError] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const epoch = useRef(0);
  const saved = Boolean(trigger.automation_id);
  useEffect(() => {
    let current = true;
    epoch.current += 1;
    setSecret(null);
    setBinding(null);
    setBusy(saved);
    if (saved)
      managePluginWebhook(trigger.automation_id, trigger.id, "get")
        .then((value) => {
          if (current) setBinding(value);
        })
        .catch(() => {
          if (current) setError(true);
        })
        .finally(() => {
          if (current) setBusy(false);
        });
    return () => {
      current = false;
      epoch.current += 1;
    };
  }, [saved, trigger.automation_id, trigger.id, trigger.updated_at, dirty]);
  const act = async (operation: "configure" | "rotate" | "reveal" | "delete") => {
    if (busy) return;
    const requestEpoch = epoch.current;
    setBusy(true);
    setError(false);
    setSecret(null);
    try {
      const value = await managePluginWebhook(trigger.automation_id, trigger.id, operation);
      if (epoch.current !== requestEpoch) return;
      setBinding(value);
      setSecret(value?.secret ?? null);
    } catch {
      if (epoch.current === requestEpoch) setError(true);
    } finally {
      if (epoch.current === requestEpoch) setBusy(false);
    }
  };
  const refresh = async () => {
    if (busy) return;
    const requestEpoch = epoch.current;
    setBusy(true);
    try {
      const rows = await listPluginWebhookReceipts(trigger.automation_id);
      if (epoch.current === requestEpoch) setReceipts(rows);
    } catch {
      if (epoch.current === requestEpoch) setError(true);
    } finally {
      if (epoch.current === requestEpoch) setBusy(false);
    }
  };
  const url = binding ? new URL(binding.path, getBackendConfig().apiBaseUrl).toString() : "";
  return { binding, receipts, busy, error, secret, saved, act, refresh, url, setError, setSecret };
}
