"use client";

import { useCallback, useEffect, useState, type Dispatch, type SetStateAction } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import { type IntegrationAuthHealth } from "@/components/integrations/auth-status-banner";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import {
  getSlackConfig,
  setSlackConfig,
  deleteSlackConfig,
  testSlackConnection,
} from "@/lib/api/domains/slack-api";
import { listUtilityAgents, type UtilityAgent } from "@/lib/api/domains/utility-api";
import type { SlackConfig, TestSlackConnectionResult } from "@/lib/types/slack";

export const DEFAULT_PREFIX = "!kandev";
export const DEFAULT_POLL_INTERVAL_SECONDS = 30;
export const MIN_POLL_INTERVAL_SECONDS = 5;
export const MAX_POLL_INTERVAL_SECONDS = 600;

/**
 * Slack credential and prompt-template tokens. Every one of these is a value the
 * user has to find or type verbatim — a cookie name, a token prefix, a
 * placeholder the backend substitutes — so they are interpolated into catalog
 * messages rather than written into them, where the pseudo-locale would
 * transliterate them into dead pointers.
 */
export const DEFAULT_PREFIX_EXAMPLE = `${DEFAULT_PREFIX} <instruction>`;
export const SLACK_TOKEN_PREFIX = "xoxc-";
export const SLACK_COOKIE_NAME = "d";
export const SLACK_TOKEN_PLACEHOLDER = "xoxc-...";
export const SLACK_COOKIE_PLACEHOLDER = "xoxd-...";
export const SECRET_MASK = "••••••••";
export const UTILITY_AGENTS_ROUTE = "/settings/utility-agents";
export const PROMPT_TOKENS = [
  "{{SlackInstruction}}",
  "{{SlackThread}}",
  "{{SlackPermalink}}",
  "{{SlackUser}}",
  "{{SlackChannelID}}",
  "{{SlackTS}}",
] as const;

export type FormState = {
  utilityAgentId: string;
  commandPrefix: string;
  pollIntervalSeconds: number;
  token: string;
  cookie: string;
};

export const emptyForm: FormState = {
  utilityAgentId: "",
  commandPrefix: DEFAULT_PREFIX,
  pollIntervalSeconds: DEFAULT_POLL_INTERVAL_SECONDS,
  token: "",
  cookie: "",
};

export function configToForm(cfg: SlackConfig | null): FormState {
  if (!cfg) return emptyForm;
  return {
    utilityAgentId: cfg.utilityAgentId,
    commandPrefix: cfg.commandPrefix || DEFAULT_PREFIX,
    pollIntervalSeconds: cfg.pollIntervalSeconds || DEFAULT_POLL_INTERVAL_SECONDS,
    token: "",
    cookie: "",
  };
}

export function configToHealth(config: SlackConfig | null): IntegrationAuthHealth | null {
  if (!config?.hasToken || !config.hasCookie) return null;
  if (!config.lastCheckedAt) return { ok: false, error: "", checkedAt: null };
  return {
    ok: !!config.lastOk,
    error: config.lastError ?? "",
    checkedAt: new Date(config.lastCheckedAt),
  };
}

function useUtilityAgentsLoader() {
  const [agents, setAgents] = useState<UtilityAgent[] | null>(null);
  useEffect(() => {
    let cancelled = false;
    listUtilityAgents({ cache: "no-store" })
      .then((res) => {
        if (!cancelled) setAgents(res.agents ?? []);
      })
      .catch(() => {
        if (!cancelled) setAgents([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);
  return { agents: agents ?? [], loadingAgents: agents === null };
}

type SettingsActionsArgs = {
  workspaceId: string;
  form: FormState;
  setConfig: (cfg: SlackConfig | null) => void;
  setForm: Dispatch<SetStateAction<FormState>>;
  setTestResult: (r: TestSlackConnectionResult | null) => void;
};

function useSettingsActions({
  workspaceId,
  form,
  setConfig,
  setForm,
  setTestResult,
}: SettingsActionsArgs) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const handleTest = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await testSlackConnection(
        {
          authMethod: "cookie",
          utilityAgentId: form.utilityAgentId,
          commandPrefix: form.commandPrefix,
          pollIntervalSeconds: form.pollIntervalSeconds,
          token: form.token || undefined,
          cookie: form.cookie || undefined,
        },
        { workspaceId },
      );
      setTestResult(res);
    } catch (err) {
      setTestResult({ ok: false, error: String(err) });
    } finally {
      setTesting(false);
    }
  }, [workspaceId, form, setTestResult]);

  const handleSave = useCallback(async () => {
    const submitted = form;
    setSaving(true);
    try {
      const saved = await setSlackConfig(
        {
          authMethod: "cookie",
          utilityAgentId: form.utilityAgentId,
          commandPrefix: form.commandPrefix,
          pollIntervalSeconds: form.pollIntervalSeconds,
          token: form.token || undefined,
          cookie: form.cookie || undefined,
        },
        { workspaceId },
      );
      setConfig(saved);
      setForm((current) =>
        JSON.stringify(current) === JSON.stringify(submitted) ? configToForm(saved) : current,
      );
      setTestResult(null);
      toast({ description: t("slack:configurationSaved"), variant: "success" });
    } catch (err) {
      toast({ description: t("slack:saveFailed", { error: String(err) }), variant: "error" });
      throw err;
    } finally {
      setSaving(false);
    }
  }, [workspaceId, form, t, toast, setConfig, setForm, setTestResult]);

  const handleDelete = useCallback(async () => {
    if (!confirm(t("slack:removeConfigurationConfirm"))) return;
    setDeleting(true);
    try {
      await deleteSlackConfig({ workspaceId });
      setConfig(null);
      setForm(emptyForm);
      setTestResult(null);
      toast({ description: t("slack:configurationRemoved"), variant: "success" });
    } catch (err) {
      toast({ description: t("slack:deleteFailed", { error: String(err) }), variant: "error" });
    } finally {
      setDeleting(false);
    }
  }, [workspaceId, t, toast, setConfig, setForm, setTestResult]);

  return { saving, testing, deleting, handleTest, handleSave, handleDelete };
}

export function useSlackSettings(workspaceId: string) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [config, setConfig] = useState<SlackConfig | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [loading, setLoading] = useState(true);
  const [testResult, setTestResult] = useState<TestSlackConnectionResult | null>(null);
  const health = configToHealth(config);
  const { agents, loadingAgents } = useUtilityAgentsLoader();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const cfg = await getSlackConfig({ workspaceId });
      setConfig(cfg);
      setForm(configToForm(cfg));
    } catch (err) {
      toast({
        description: t("slack:failedToLoadConfig", { error: String(err) }),
        variant: "error",
      });
    } finally {
      setLoading(false);
    }
  }, [workspaceId, t, toast]);

  useEffect(() => {
    void load();
  }, [load]);

  // Background refresh so the auth-health banner picks up new probe results.
  useEffect(() => {
    const id = setInterval(() => {
      getSlackConfig({ workspaceId })
        .then((cfg) => setConfig(cfg))
        .catch(() => {
          /* transient failures are fine — next tick retries */
        });
    }, INTEGRATION_STATUS_REFRESH_MS);
    return () => clearInterval(id);
  }, [workspaceId]);

  const update = useCallback(
    <K extends keyof FormState>(key: K, value: FormState[K]) =>
      setForm((prev) => ({ ...prev, [key]: value })),
    [],
  );
  const discard = useCallback(() => setForm(configToForm(config)), [config]);

  const { saving, testing, deleting, handleTest, handleSave, handleDelete } = useSettingsActions({
    workspaceId,
    form,
    setConfig,
    setForm,
    setTestResult,
  });

  return {
    config,
    form,
    loading,
    saving,
    testing,
    deleting,
    testResult,
    health,
    agents,
    loadingAgents,
    update,
    discard,
    handleTest,
    handleSave,
    handleDelete,
  };
}
