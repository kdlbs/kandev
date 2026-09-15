import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { PluginWebhookControls } from "./plugin-webhook-controls";
import type { AutomationTrigger, TriggerTypeInfo } from "@/lib/types/automation";
import { conditionSettings } from "../plugin-condition";

export function PluginEventConfig({
  trigger,
  info,
  onUpdate,
  dirty,
}: {
  dirty: boolean;
  trigger: AutomationTrigger;
  info?: NonNullable<TriggerTypeInfo["plugin"]>;
  onUpdate: (config: Record<string, unknown>) => void;
}) {
  const { t } = useTranslation();
  const compatible = info?.condition.config_version === trigger.config.config_version;
  const settings = conditionSettings(trigger.config);
  const properties = (compatible ? (info?.condition.config_schema.properties ?? {}) : {}) as Record<
    string,
    Record<string, unknown>
  >;
  const label = (key: string, fallback: string) =>
    t(key, { ns: `plugin-${info?.plugin_id}`, defaultValue: fallback });
  return (
    <div className="space-y-4 pt-3">
      {info && (
        <p className="font-medium">
          {label(info.condition.label_key ?? info.condition.label, info.condition.label)}
        </p>
      )}
      {(!info?.available || !compatible) && (
        <p role="status">{t("automations:pluginWebhookUnavailable")}</p>
      )}
      {Object.entries(properties).map(([name, schema]) => {
        const id = `${trigger.id}-${name}`;
        const change = (value: unknown) =>
          onUpdate({ ...trigger.config, settings: { ...settings, [name]: value } });
        return (
          <div key={name} className="space-y-2">
            <Label htmlFor={id}>
              {label(String(schema.title_key ?? name), String(schema.title ?? name))}
            </Label>
            <ConditionField
              id={id}
              schema={schema}
              value={settings[name]}
              onChange={change}
              options={info?.config_options?.[name]}
            />
          </div>
        );
      })}
      <PluginWebhookControls
        key={`${trigger.id}:${trigger.updated_at}:${dirty}`}
        trigger={trigger}
        dirty={dirty}
      />
    </div>
  );
}

function ConditionField({
  id,
  schema,
  value,
  onChange,
  options,
}: {
  options?: string[];
  id: string;
  schema: Record<string, unknown>;
  value: unknown;
  onChange: (value: unknown) => void;
}) {
  const { t } = useTranslation();
  if (schema.type === "array") return <StringListField id={id} value={value} onChange={onChange} />;
  if (schema.type === "boolean")
    return <Switch id={id} checked={value === true} onCheckedChange={onChange} />;
  if (Array.isArray(schema.enum))
    return (
      <select
        id={id}
        className="w-full h-7 rounded-md border bg-background px-2 text-xs [@media(pointer:coarse)]:min-h-11"
        value={String(value ?? "")}
        onChange={(event) => onChange(event.target.value)}
      >
        <option value="">{t("automations:pluginWebhookChoose")}</option>
        {schema.enum.map((option) => (
          <option key={String(option)} value={String(option)}>
            {String(option)}
          </option>
        ))}
      </select>
    );
  return (
    <>
      <Input
        id={id}
        list={options ? `${id}-options` : undefined}
        value={String(value ?? "")}
        onChange={(event) => onChange(event.target.value)}
      />
      {options && (
        <datalist id={`${id}-options`}>
          {options.map((option) => (
            <option key={option} value={option} />
          ))}
        </datalist>
      )}
    </>
  );
}

function StringListField({
  id,
  value,
  onChange,
}: {
  id: string;
  value: unknown;
  onChange: (value: unknown) => void;
}) {
  const serialized = Array.isArray(value) ? value.join("\n") : "";
  const [draft, setDraft] = useState(serialized);
  useEffect(() => setDraft(serialized), [serialized]);
  return (
    <Textarea
      id={id}
      value={draft}
      onChange={(event) => setDraft(event.target.value)}
      onBlur={() =>
        onChange(
          draft
            .split("\n")
            .map((item) => item.trim())
            .filter(Boolean),
        )
      }
    />
  );
}
