"use client";

import { IconX } from "@tabler/icons-react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Input } from "@kandev/ui/input";
import { Button } from "@kandev/ui/button";
import type {
  FilterClause,
  FilterDimension,
  FilterOp,
  FilterValue,
} from "@/lib/state/slices/ui/sidebar-view-types";
import { DIMENSION_METAS, OP_LABELS, getDimensionMeta } from "./filter-dimension-registry";
import { useFilterValueOptions } from "./use-filter-value-options";

type Props = {
  clause: FilterClause;
  onChange: (next: FilterClause) => void;
  onRemove: () => void;
};

function normaliseValueForDimension(
  value: FilterValue,
  meta: ReturnType<typeof getDimensionMeta>,
  op: FilterOp,
): FilterValue {
  if (meta.valueKind === "boolean") {
    if (typeof value === "boolean") return value;
    return meta.defaultValue as boolean;
  }
  if (meta.valueKind === "enum") {
    const multi = op === "in" || op === "not_in";
    if (multi) {
      if (Array.isArray(value)) return value;
      return value ? [String(value)] : [];
    }
    return Array.isArray(value) ? (value[0] ?? "") : String(value);
  }
  return Array.isArray(value) ? (value[0] ?? "") : String(value);
}

export function FilterClauseEditor({ clause, onChange, onRemove }: Props) {
  const meta = getDimensionMeta(clause.dimension);
  const enumOptions = useFilterValueOptions(clause.dimension);
  const availableOptions = meta.enumOptions ?? enumOptions;

  function handleDimensionChange(next: FilterDimension) {
    const nextMeta = getDimensionMeta(next);
    onChange({
      ...clause,
      dimension: next,
      op: nextMeta.defaultOp,
      value: nextMeta.defaultValue,
    });
  }

  function handleOpChange(next: FilterOp) {
    onChange({
      ...clause,
      op: next,
      value: normaliseValueForDimension(clause.value, meta, next),
    });
  }

  function handleValueChange(next: FilterValue) {
    onChange({ ...clause, value: next });
  }

  return (
    <div
      className="flex items-center gap-1.5 py-1"
      data-testid="filter-clause-row"
      data-clause-id={clause.id}
    >
      <Select
        value={clause.dimension}
        onValueChange={(v) => handleDimensionChange(v as FilterDimension)}
      >
        <SelectTrigger
          size="sm"
          className="h-7 flex-1 text-xs"
          data-testid="filter-dimension-select"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {DIMENSION_METAS.map((m) => (
            <SelectItem key={m.dimension} value={m.dimension} className="text-xs">
              {m.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select value={clause.op} onValueChange={(v) => handleOpChange(v as FilterOp)}>
        <SelectTrigger size="sm" className="h-7 w-24 text-xs" data-testid="filter-op-select">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {meta.ops.map((op) => (
            <SelectItem key={op} value={op} className="text-xs">
              {OP_LABELS[op]}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <ValueInput clause={clause} options={availableOptions} onChange={handleValueChange} />

      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="h-6 w-6 cursor-pointer text-muted-foreground hover:text-foreground"
        onClick={onRemove}
        data-testid="filter-clause-remove"
        aria-label="Remove filter"
      >
        <IconX className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

function ValueInput({
  clause,
  options,
  onChange,
}: {
  clause: FilterClause;
  options: Array<{ value: string; label: string }>;
  onChange: (v: FilterValue) => void;
}) {
  const meta = getDimensionMeta(clause.dimension);

  if (meta.valueKind === "boolean") {
    const current = clause.value === true ? "true" : "false";
    return (
      <Select value={current} onValueChange={(v) => onChange(v === "true")}>
        <SelectTrigger size="sm" className="h-7 w-20 text-xs" data-testid="filter-value-select">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="true" className="text-xs">
            true
          </SelectItem>
          <SelectItem value="false" className="text-xs">
            false
          </SelectItem>
        </SelectContent>
      </Select>
    );
  }

  if (meta.valueKind === "text") {
    return (
      <Input
        value={String(clause.value ?? "")}
        onChange={(e) => onChange(e.target.value)}
        placeholder={meta.placeholder ?? "Value"}
        className="h-7 flex-1 text-xs"
        data-testid="filter-value-input"
      />
    );
  }

  const multi = clause.op === "in" || clause.op === "not_in";

  if (multi) {
    const selected = new Set(Array.isArray(clause.value) ? clause.value.map(String) : []);
    return (
      <div
        className="flex h-7 flex-1 flex-wrap items-center gap-0.5 overflow-hidden rounded-md border bg-transparent px-1"
        data-testid="filter-value-multi"
      >
        {options.map((opt) => {
          const active = selected.has(opt.value);
          return (
            <button
              type="button"
              key={opt.value}
              onClick={() => {
                const next = new Set(selected);
                if (active) next.delete(opt.value);
                else next.add(opt.value);
                onChange([...next]);
              }}
              className={`cursor-pointer rounded px-1.5 text-[10px] ${
                active
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:text-foreground"
              }`}
              data-testid="filter-value-multi-option"
              data-value={opt.value}
              data-active={active}
            >
              {opt.label}
            </button>
          );
        })}
      </div>
    );
  }

  const current = String(clause.value ?? "");
  return (
    <Select value={current} onValueChange={(v) => onChange(v)}>
      <SelectTrigger size="sm" className="h-7 flex-1 text-xs" data-testid="filter-value-select">
        <SelectValue placeholder="Select value" />
      </SelectTrigger>
      <SelectContent>
        {options.length === 0 && (
          <SelectItem value="__empty__" disabled className="text-xs">
            No options
          </SelectItem>
        )}
        {options.map((opt) => (
          <SelectItem key={opt.value} value={opt.value} className="text-xs">
            {opt.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
