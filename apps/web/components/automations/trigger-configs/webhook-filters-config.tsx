"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconPlus, IconTrash } from "@tabler/icons-react";
import type { WebhookFilter, WebhookFilterOp } from "@/lib/types/automation";

// Every operator EvaluateFilters understands (internal/automation/models.go).
// `values` is meaningless for exists/not_exists — S6's cardinality rule.
const FILTER_OPS: WebhookFilterOp[] = [
  "eq",
  "ne",
  "in",
  "not_in",
  "exists",
  "not_exists",
  "contains",
];
const FILTER_OP_LABEL_KEYS: Record<WebhookFilterOp, string> = {
  eq: "automations:webhookFilterOpEq",
  ne: "automations:webhookFilterOpNe",
  in: "automations:webhookFilterOpIn",
  not_in: "automations:webhookFilterOpNotIn",
  exists: "automations:webhookFilterOpExists",
  not_exists: "automations:webhookFilterOpNotExists",
  contains: "automations:webhookFilterOpContains",
};
const FILTER_OPS_WITHOUT_VALUES = new Set<WebhookFilterOp>(["exists", "not_exists"]);

type WebhookFiltersConfigProps = {
  filters: WebhookFilter[];
  onChange: (next: WebhookFilter[]) => void;
};

export function WebhookFiltersConfig({ filters, onChange }: WebhookFiltersConfigProps) {
  const { t } = useTranslation();

  const updateFilter = (index: number, next: WebhookFilter) => {
    const copy = filters.slice();
    copy[index] = next;
    onChange(copy);
  };
  const removeFilter = (index: number) => onChange(filters.filter((_, i) => i !== index));
  const addFilter = () => onChange([...filters, { path: "", op: "eq", values: [] }]);

  return (
    <div className="space-y-2">
      <Label className="text-xs">{t("automations:webhookFiltersLabel")}</Label>
      <p className="text-xs text-muted-foreground">{t("automations:webhookFiltersHelp")}</p>
      {filters.map((filter, index) => (
        // No persisted identity to key on — filters are an ordered list edited
        // in place, never reordered, so the index is a stable-enough key.
        <FilterRow
          key={index}
          filter={filter}
          onChange={(next) => updateFilter(index, next)}
          onRemove={() => removeFilter(index)}
        />
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="cursor-pointer"
        onClick={addFilter}
      >
        <IconPlus className="mr-1.5 h-3.5 w-3.5" />
        {t("automations:addFilter")}
      </Button>
    </div>
  );
}

function FilterRow({
  filter,
  onChange,
  onRemove,
}: {
  filter: WebhookFilter;
  onChange: (next: WebhookFilter) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const needsValues = !FILTER_OPS_WITHOUT_VALUES.has(filter.op);
  return (
    <div className="flex items-start gap-2">
      {/* An example JSON payload path — data the user types verbatim, not copy. */}
      <Input
        value={filter.path}
        onChange={(e) => onChange({ ...filter, path: e.target.value })}
        className="font-mono text-xs"
        // eslint-disable-next-line i18next/no-literal-string -- example payload path, see above
        placeholder="severity"
      />
      <Select
        value={filter.op}
        onValueChange={(op) => {
          const nextOp = op as WebhookFilterOp;
          onChange({
            ...filter,
            op: nextOp,
            values: FILTER_OPS_WITHOUT_VALUES.has(nextOp) ? [] : filter.values,
          });
        }}
      >
        <SelectTrigger className="cursor-pointer w-[140px] shrink-0">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {FILTER_OPS.map((op) => (
            <SelectItem key={op} value={op}>
              {t(FILTER_OP_LABEL_KEYS[op])}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {needsValues ? (
        <FilterValuesInput
          values={filter.values ?? []}
          onChange={(values) => onChange({ ...filter, values })}
        />
      ) : (
        <div className="flex-1" />
      )}
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        className="cursor-pointer text-muted-foreground hover:text-destructive shrink-0"
        onClick={onRemove}
        title={t("automations:removeFilter")}
      >
        <IconTrash className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

// Buffers the comma-separated text locally so a trailing ", " mid-edit isn't
// immediately reformatted away; committed as a trimmed string array on blur.
// Mirrors GitHubPRConfig's branches/authors fields.
//
// A lone trailing segment produced only by an in-progress trailing comma is
// dropped (so "critical, fatal ," commits as ["critical", "fatal"]), but an
// otherwise-empty segment is kept — so typing "," commits an explicit single
// empty-string value (values: [""]), which some operators such as `ne`
// legitimately need. A field that was never edited, or holds only
// whitespace, commits as an empty array rather than [""].
function commitFilterValues(text: string): string[] {
  if (text.trim() === "") {
    return [];
  }
  const parts = text.split(",").map((v) => v.trim());
  if (parts.length > 1 && parts[parts.length - 1] === "") {
    return parts.slice(0, -1);
  }
  return parts;
}

function FilterValuesInput({
  values,
  onChange,
}: {
  values: string[];
  onChange: (values: string[]) => void;
}) {
  const joined = values.join(", ");
  const [text, setText] = useState(joined);
  useEffect(() => setText(joined), [joined]);

  return (
    <Input
      value={text}
      onChange={(e) => setText(e.target.value)}
      onBlur={() => onChange(commitFilterValues(text))}
      className="font-mono text-xs"
      // eslint-disable-next-line i18next/no-literal-string -- example comparison values, not copy
      placeholder="critical, fatal"
    />
  );
}
