"use client";

import { useMemo } from "react";
import { IconArrowUp, IconArrowDown, IconPlus, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useTranslation } from "react-i18next";

type Props = {
  order: string[];
  knownProviders: string[];
  onChange: (next: string[]) => void;
  disabled?: boolean;
};

export function ProviderOrderEditor({ order, knownProviders, onChange, disabled }: Props) {
  const { t } = useTranslation();
  const remaining = useMemo(
    () => knownProviders.filter((p) => !order.includes(p)),
    [knownProviders, order],
  );

  const move = (idx: number, delta: number) => {
    const j = idx + delta;
    if (j < 0 || j >= order.length) return;
    const next = [...order];
    [next[idx], next[j]] = [next[j], next[idx]];
    onChange(next);
  };

  const remove = (id: string) => onChange(order.filter((p) => p !== id));
  const add = (id: string) => onChange([...order, id]);

  return (
    <div className="rounded-lg border border-border p-4 space-y-3">
      <div>
        <p className="text-sm font-medium">{t("office:providerOrder")}</p>
        <p className="text-xs text-muted-foreground mt-0.5">
          {t("office:topToBottomFallbackOrderAgents")}
        </p>
      </div>
      <ul className="space-y-1.5" data-testid="provider-order-list">
        {order.length === 0 && (
          <li className="text-xs text-muted-foreground italic">
            {t("office:noProvidersSelected")}
          </li>
        )}
        {order.map((p, idx) => (
          <li
            key={p}
            className="flex items-center gap-2 rounded border border-border bg-background px-2 py-1.5"
          >
            <span className="text-xs text-muted-foreground tabular-nums w-5">{idx + 1}.</span>
            <span className="text-sm flex-1">{providerLabel(p)}</span>
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6 cursor-pointer"
              onClick={() => move(idx, -1)}
              disabled={disabled || idx === 0}
              aria-label={t("office:moveUp")}
            >
              <IconArrowUp className="h-3.5 w-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6 cursor-pointer"
              onClick={() => move(idx, 1)}
              disabled={disabled || idx === order.length - 1}
              aria-label={t("office:moveDown")}
            >
              <IconArrowDown className="h-3.5 w-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="h-6 w-6 cursor-pointer text-destructive"
              onClick={() => remove(p)}
              disabled={disabled}
              aria-label={t("office:remove")}
            >
              <IconX className="h-3.5 w-3.5" />
            </Button>
          </li>
        ))}
      </ul>
      {remaining.length > 0 && (
        <AddProviderRow remaining={remaining} onAdd={add} disabled={disabled} />
      )}
    </div>
  );
}

function AddProviderRow({
  remaining,
  onAdd,
  disabled,
}: {
  remaining: string[];
  onAdd: (id: string) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2 pt-1">
      <Select
        value=""
        onValueChange={(v) => v && onAdd(v)}
        disabled={disabled || remaining.length === 0}
      >
        <SelectTrigger className="w-[220px] cursor-pointer" data-testid="provider-add-select">
          <SelectValue placeholder={t("office:addProvider")} />
        </SelectTrigger>
        <SelectContent>
          {remaining.map((p) => (
            <SelectItem key={p} value={p} className="cursor-pointer">
              {providerLabel(p)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <span className="text-xs text-muted-foreground">
        <IconPlus className="inline h-3 w-3 mr-0.5" />
        {t("office:addToOrder")}
      </span>
    </div>
  );
}

export function providerLabel(id: string): string {
  return PROVIDER_LABELS[id] ?? id;
}

// i18n-exempt: agent product names.
const PROVIDER_LABELS: Record<string, string> = {
  "claude-acp": "Claude (ACP)",
  "codex-acp": "Codex (ACP)",
  "opencode-acp": "OpenCode (ACP)",
  "copilot-acp": "GitHub Copilot",
  "amp-acp": "Sourcegraph Amp",
};
