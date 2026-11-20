"use client";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Checkbox } from "@kandev/ui/checkbox";
import { Input } from "@kandev/ui/input";
import { Button } from "@kandev/ui/button";
import type { AutomationMCPOptions, AutomationMCPSelection } from "@/lib/types/automation";

type FieldsProps = {
  options?: AutomationMCPOptions;
  value: AutomationMCPSelection;
  onChange: (value: AutomationMCPSelection) => void;
  touch: boolean;
  disabled: boolean;
};
const rowClass = (touch: boolean) =>
  `flex items-start gap-3 py-2 ${touch ? "min-h-11" : "min-h-7"}`;

/**
 * Separate inherited servers from explicit selections and retain controls for removing
 * unavailable references.
 */
export function AutomationWorkspaceFields({
  options,
  value,
  onChange,
  touch,
  disabled,
}: FieldsProps) {
  const { t } = useTranslation();
  const [search, setSearch] = useState("");
  const inherited = new Set(options?.inherited.map((item) => item.id));
  const definitions =
    options?.definitions.filter(
      (item) =>
        (!inherited.has(item.id) || value.definition_ids.includes(item.id)) &&
        item.name.toLocaleLowerCase().includes(search.toLocaleLowerCase()),
    ) ?? [];
  const toggle = (id: string, checked: boolean) =>
    onChange({
      ...value,
      definition_ids: checked
        ? [...value.definition_ids, id]
        : value.definition_ids.filter((item) => item !== id),
    });
  const unavailable = value.definition_ids.filter(
    (id) => !options?.definitions.some((item) => item.id === id && item.enabled),
  );
  return (
    <div className="min-w-0 space-y-2">
      <fieldset disabled={disabled} className="min-w-0 space-y-2">
        <legend className="text-sm font-medium">{t("automations:mcpWorkspaceServers")}</legend>
        <Input
          className={touch ? "min-h-11" : "h-7"}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder={t("automations:mcpSearch")}
          aria-label={t("automations:mcpSearch")}
        />
        {options?.inherited.map((item) => (
          <p key={item.id} className="break-words text-xs text-muted-foreground">
            {t("automations:mcpInherited", { name: item.name })} (
            {item.origins
              .map((origin) =>
                origin === "profile"
                  ? t("settings:mcpOriginProfile")
                  : t("settings:mcpOriginRepository"),
              )
              .join(", ")}
            )
          </p>
        ))}
        <div className="max-h-64 space-y-1 overflow-y-auto overscroll-contain">
          {definitions.map((item) => (
            <label
              key={item.id}
              className={rowClass(touch)}
              data-testid={`automation-mcp-server-${item.id}`}
            >
              <Checkbox
                checked={value.definition_ids.includes(item.id)}
                disabled={!item.enabled}
                onCheckedChange={(checked) => toggle(item.id, checked === true)}
              />
              <span className="min-w-0 break-words text-sm">{item.name}</span>
            </label>
          ))}
        </div>
        {!disabled && definitions.length === 0 ? (
          <p className="text-xs text-muted-foreground">{t("automations:mcpNoServers")}</p>
        ) : null}
      </fieldset>
      {!disabled ? (
        <AutomationUnavailableServers
          ids={unavailable}
          touch={touch}
          remove={(id) => toggle(id, false)}
        />
      ) : null}
    </div>
  );
}
/**
 * Let users explicitly remove saved references that can no longer attach.
 */
function AutomationUnavailableServers({
  ids,
  touch,
  remove,
}: {
  ids: string[];
  touch: boolean;
  remove: (id: string) => void;
}) {
  const { t } = useTranslation();
  return ids.map((id) => (
    <div key={id} className="flex min-w-0 items-center justify-between gap-2 text-xs">
      <span className="min-w-0 break-all">{t("automations:mcpUnavailable", { name: id })}</span>
      <Button
        type="button"
        size="sm"
        variant="outline"
        className={touch ? "min-h-11" : "h-7"}
        onClick={() => remove(id)}
      >
        {t("settings:remove")}
      </Button>
    </div>
  ));
}
