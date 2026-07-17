"use client";

import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { getPluginConfig, updatePluginConfig } from "@/lib/api/domains/plugins-api";
import {
  buildInitialValues,
  missingRequiredFields,
  parseConfigSchema,
  serializeConfigValues,
} from "@/lib/plugins/config-schema";
import type { PluginRecord } from "@/lib/types/plugins";

type SaveStatus = "idle" | "loading" | "success" | "error";
type FormValues = Record<string, string | boolean>;

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

  const handleChange = (name: string, value: string | boolean) => {
    setValues((prev) => ({ ...prev, [name]: value }));
    setSaveStatus("idle");
  };

  const handleSave = async () => {
    if (!pluginId) return;
    const missing = missingRequiredFields(fields, values);
    if (missing.length > 0) {
      toast.error(`Required: ${missing.join(", ")}`);
      return;
    }
    setSaveStatus("loading");
    try {
      await updatePluginConfig(pluginId, serializeConfigValues(fields, values));
      const refreshed = await getPluginConfig(pluginId, { cache: "no-store" });
      const initial = buildInitialValues(fields, refreshed);
      setValues(initial);
      setInitialValues(initial);
      setSaveStatus("success");
      toast.success("Plugin settings saved");
    } catch (err) {
      setSaveStatus("error");
      toast.error(err instanceof Error ? err.message : "Failed to save plugin settings");
    }
  };

  return {
    fields,
    values,
    configLoading,
    configError,
    saveStatus,
    isDirty,
    handleChange,
    handleSave,
  };
}
