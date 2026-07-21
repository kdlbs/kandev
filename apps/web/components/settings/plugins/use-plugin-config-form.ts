"use client";

import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { getPluginConfig, updatePluginConfig } from "@/lib/api/domains/plugins-api";
import {
  SECRET_MASK,
  buildInitialValues,
  missingRequiredFields,
  parseConfigSchema,
  serializeConfigValues,
} from "@/lib/plugins/config-schema";
import type { PluginRecord } from "@/lib/types/plugins";

type SaveStatus = "idle" | "loading" | "success" | "error";
type FormValues = Record<string, string | boolean>;

function maskSecretsIn(
  source: FormValues,
  fields: ReturnType<typeof parseConfigSchema>,
): FormValues {
  const masked = { ...source };
  for (const field of fields) {
    const current = masked[field.name];
    if (field.secret && typeof current === "string" && current !== "") {
      masked[field.name] = SECRET_MASK;
    }
  }
  return masked;
}

/**
 * Load/edit/save state for one plugin's schema-driven settings form.
 * Mirrors use-plugin-actions' local-hook pattern: fetch + toast wiring lives
 * here, the components stay presentational. Saving PATCHes the full config
 * (secret fields carrying the mask keep their stored value server-side) and
 * then re-fetches the masked config so the form reflects what is stored.
 */
export function usePluginConfigForm(plugin: PluginRecord | null) {
  const fields = useMemo(() => parseConfigSchema(plugin?.config_schema), [plugin?.config_schema]);
  const [values, setValues] = useState<FormValues>({});
  const [initialValues, setInitialValues] = useState<FormValues>({});
  const [configLoading, setConfigLoading] = useState(false);
  const [configError, setConfigError] = useState<string | null>(null);
  const [saveStatus, setSaveStatus] = useState<SaveStatus>("idle");

  const pluginId = plugin?.id ?? null;
  const hasFields = fields.length > 0;

  useEffect(() => {
    if (!pluginId || !hasFields) return;
    let cancelled = false;
    setConfigLoading(true);
    setConfigError(null);
    getPluginConfig(pluginId, { cache: "no-store" })
      .then((config) => {
        if (cancelled) return;
        const initial = buildInitialValues(fields, config);
        setValues(initial);
        setInitialValues(initial);
      })
      .catch((err) => {
        if (!cancelled) {
          setConfigError(err instanceof Error ? err.message : "Failed to load plugin settings");
        }
      })
      .finally(() => {
        if (!cancelled) setConfigLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // fields is derived solely from plugin.config_schema; pluginId is the
    // real reload trigger.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pluginId, hasFields]);

  const isDirty = useMemo(
    () => fields.some((field) => values[field.name] !== initialValues[field.name]),
    [fields, values, initialValues],
  );
  const missing = useMemo(() => missingRequiredFields(fields, values), [fields, values]);

  const handleChange = (name: string, value: string | boolean) => {
    setValues((prev) => ({ ...prev, [name]: value }));
    setSaveStatus("idle");
  };

  const handleSave = async () => {
    if (!pluginId) return;
    if (missing.length > 0) {
      toast.error(`Required: ${missing.join(", ")}`);
      throw new Error(`Required: ${missing.join(", ")}`);
    }
    setSaveStatus("loading");
    try {
      await updatePluginConfig(pluginId, serializeConfigValues(fields, values));
    } catch (err) {
      setSaveStatus("error");
      toast.error(err instanceof Error ? err.message : "Failed to save plugin settings");
      throw err;
    }
    // The config IS persisted from here on — a refetch failure (e.g. a
    // transient hiccup while the plugin restarts) must not be reported as a
    // save failure, and the typed cleartext secret must not stay on screen.
    try {
      const refreshed = await getPluginConfig(pluginId, { cache: "no-store" });
      const initial = buildInitialValues(fields, refreshed);
      setValues(initial);
      setInitialValues(initial);
      toast.success("Plugin settings saved");
    } catch {
      const masked = maskSecretsIn(values, fields);
      setValues(masked);
      setInitialValues(masked);
      toast.warning("Settings saved, but reloading them failed — refresh to confirm.");
    }
    setSaveStatus("success");
  };

  return {
    fields,
    values,
    initialValues,
    configLoading,
    configError,
    saveStatus,
    isDirty,
    canSave: missing.length === 0,
    invalidReason: missing.length > 0 ? `Required: ${missing.join(", ")}` : undefined,
    revision: JSON.stringify(values),
    handleChange,
    handleSave,
    discard: () => {
      setValues(initialValues);
      setSaveStatus("idle");
    },
  };
}
