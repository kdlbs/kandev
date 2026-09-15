import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import {
  managePluginWebhook,
  listPluginWebhookReceipts,
  type PluginWebhookBinding,
  type PluginWebhookReceipt,
} from "@/lib/api/domains/automation-api";
import type { AutomationTrigger } from "@/lib/types/automation";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";

function useWebhookControls(trigger: AutomationTrigger, dirty: boolean) {
  const [binding, setBinding] = useState<PluginWebhookBinding | null>(null);
  const [receipts, setReceipts] = useState<PluginWebhookReceipt[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const saved = Boolean(trigger.automation_id);
  useEffect(() => {
    let current = true;
    setSecret(null);
    setBinding(null);
    if (saved)
      managePluginWebhook(trigger.automation_id, trigger.id, "get")
        .then((value) => {
          if (current) setBinding(value);
        })
        .catch(() => {
          if (current) setError(true);
        });
    return () => {
      current = false;
    };
  }, [saved, trigger.automation_id, trigger.id, trigger.updated_at, dirty]);
  const act = async (operation: "configure" | "rotate" | "reveal" | "delete") => {
    setBusy(true);
    setError(false);
    setSecret(null);
    try {
      const value = await managePluginWebhook(trigger.automation_id, trigger.id, operation);
      setBinding(value);
      setSecret(value?.secret ?? null);
    } catch {
      setError(true);
    } finally {
      setBusy(false);
    }
  };
  const refresh = async () => {
    setBusy(true);
    try {
      setReceipts(await listPluginWebhookReceipts(trigger.automation_id));
    } catch {
      setError(true);
    } finally {
      setBusy(false);
    }
  };
  const url = binding ? new URL(binding.path, window.location.origin).toString() : "";
  return { binding, receipts, busy, error, secret, saved, act, refresh, url, setError, setSecret };
}

export function PluginWebhookControls({
  trigger,
  dirty,
}: {
  trigger: AutomationTrigger;
  dirty: boolean;
}) {
  const { t } = useTranslation();
  const { binding, receipts, busy, error, secret, saved, act, refresh, url, setError, setSecret } =
    useWebhookControls(trigger, dirty);
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">{t("automations:pluginWebhookHelp")}</p>
      {error && <p role="alert">{t("automations:pluginWebhookError")}</p>}
      <div className="flex flex-wrap gap-2">
        <Button type="button" disabled={!saved || busy || dirty} onClick={() => act("configure")}>
          {t("automations:pluginWebhookConfigure")}
        </Button>
        {binding && (
          <>
            <Button type="button" variant="outline" disabled={busy} onClick={() => act("reveal")}>
              {t("automations:pluginWebhookReveal")}
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={busy || dirty}
              onClick={() => act("rotate")}
            >
              {t("automations:pluginWebhookRotate")}
            </Button>
            <Button type="button" variant="outline" disabled={busy} onClick={() => act("delete")}>
              {t("automations:pluginWebhookRevoke")}
            </Button>
          </>
        )}
      </div>
      {binding && (
        <>
          <Label htmlFor={`${trigger.id}-url`}>{t("automations:pluginWebhookURL")}</Label>
          <Input id={`${trigger.id}-url`} value={url} readOnly />
          <Button
            type="button"
            variant="outline"
            onClick={() => copyToClipboard(url).catch(() => setError(true))}
          >
            {t("automations:pluginWebhookCopy")}
          </Button>
        </>
      )}
      {secret && (
        <div>
          <Label htmlFor={`${trigger.id}-secret`}>{t("automations:pluginWebhookSecret")}</Label>
          <Input id={`${trigger.id}-secret`} value={secret} readOnly />
          <Button type="button" variant="outline" onClick={() => setSecret(null)}>
            {t("automations:pluginWebhookHide")}
          </Button>
        </div>
      )}
      {saved && (
        <Button type="button" variant="outline" disabled={busy} onClick={refresh}>
          {t("automations:pluginWebhookDeliveries")}
        </Button>
      )}
      {receipts.map((receipt) => (
        <p key={receipt.id} className="text-sm">
          {new Date(receipt.created_at * 1000).toLocaleString()}:{" "}
          {t(`automations:pluginWebhookState_${receipt.state}`)}{" "}
          {receipt.task_id ? (
            <a className="underline" href={`/tasks/${receipt.task_id}`}>
              {receipt.run_id}
            </a>
          ) : (
            receipt.run_id
          )}
          {receipt.reason && <span className="text-muted-foreground"> ({receipt.reason})</span>}
        </p>
      ))}
    </div>
  );
}
