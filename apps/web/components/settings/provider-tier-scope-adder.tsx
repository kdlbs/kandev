"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import type { RequestScope } from "@/lib/api/domains/provider-limits-api";
import { newOwnerScope } from "@/lib/settings/provider-tier-limits";
import { settingsActionClassName, settingsControlClassName } from "./settings-control";

type Props = {
  prefix: string;
  models: string[];
  existing: RequestScope[];
  onAdd: (scope: RequestScope) => void;
};

/** Adds an owner scope: one selected model, or a shared group of several. */
export function ProviderTierScopeAdder({ prefix, models, existing, onAdd }: Props) {
  const { t } = useTranslation();
  const [selected, setSelected] = useState<string[]>([]);
  const [value, setValue] = useState("");
  if (models.length === 0) return null;
  const candidate = selected.length > 0 ? newOwnerScope(selected, undefined) : undefined;
  const duplicate = candidate ? existing.some((scope) => scope.id === candidate.id) : false;
  const toggle = (model: string) =>
    setSelected((current) =>
      current.includes(model) ? current.filter((m) => m !== model) : [...current, model],
    );
  const add = () => {
    if (!candidate || duplicate) return;
    onAdd(newOwnerScope(selected, value === "" ? undefined : Number(value)));
    setSelected([]);
    setValue("");
  };
  return (
    <div className="grid min-w-0 gap-1 border-t pt-2">
      <p className="text-xs text-muted-foreground">{t("agents:providerScopeAddHint")}</p>
      <div
        className="flex flex-wrap gap-1"
        role="group"
        aria-label={t("agents:providerScopeModels")}
      >
        {models.map((model) => (
          <Button
            key={model}
            type="button"
            aria-pressed={selected.includes(model)}
            variant={selected.includes(model) ? "secondary" : "outline"}
            className={settingsActionClassName("max-w-full break-all")}
            onClick={() => toggle(model)}
          >
            {model}
          </Button>
        ))}
      </div>
      <Label htmlFor={`${prefix}-scope-new-value`}>{t("agents:providerScopeNewValue")}</Label>
      <Input
        id={`${prefix}-scope-new-value`}
        className={settingsControlClassName("w-full")}
        type="number"
        min={0}
        step="any"
        value={value}
        onChange={(event) => setValue(event.target.value)}
      />
      <Button
        type="button"
        variant="outline"
        className={settingsActionClassName("justify-self-start")}
        disabled={!candidate || duplicate}
        onClick={add}
      >
        {t(
          candidate?.kind === "group" ? "agents:providerScopeAddGroup" : "agents:providerScopeAdd",
        )}
      </Button>
    </div>
  );
}
