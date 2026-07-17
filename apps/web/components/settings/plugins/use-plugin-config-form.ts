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
    } catch (err) {
      setSaveStatus("error");
      toast.error(err instanceof Error ? err.message : "Failed to save plugin settings");
      return;
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
      maskSecretValues();
      toast.warning("Settings saved, but reloading them failed — refresh to confirm.");
    }
    setSaveStatus("success");
  };

  // On a post-save refetch failure, replace any typed secret input with the
  // mask (so cleartext never lingers) and rebase initialValues onto the
  // masked snapshot — the config IS saved, so the masked form is the new
  // baseline and must not read as dirty (e.g. a previously-unset secret the
  // user just entered). Computed from the current `values` and applied to
  // both state setters directly, rather than as a side effect inside a
  // functional updater (which would run twice under StrictMode).
  const maskSecretValues = () => {
    const masked = maskSecretsIn(values);
    setValues(masked);
    setInitialValues(masked);
  };

  const maskSecretsIn = (source: FormValues): FormValues => {
    const masked = { ...source };
    for (const field of fields) {
      const current = masked[field.name];
      if (field.secret && typeof current === "string" && current !== "") {
        masked[field.name] = SECRET_MASK;
      }
    }
    return masked;
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
