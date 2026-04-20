"use client";

import { IconX } from "@tabler/icons-react";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@kandev/ui/select";
import { Fragment } from "react";
import { Input } from "@kandev/ui/input";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import type {
  FilterClause,
  FilterDimension,
  FilterOp,
  FilterValue,
} from "@/lib/state/slices/ui/sidebar-view-types";
import { DIMENSION_METAS, getDimensionMeta, getOpLabel } from "./filter-dimension-registry";
import { useFilterValueOptions } from "./use-filter-value-options";
import { FilterMultiSelect } from "./filter-multi-select";
import { buildOptionGroups, hasGroupedOptions } from "./filter-option-groups";

type ValueOption = { value: string; label: string; color?: string; group?: string };

function OptionLabel({ option }: { option: ValueOption }) {
  return (
    <span className="flex items-center gap-1.5">
      {option.color && <span className={cn("block h-2 w-2 shrink-0 rounded-full", option.color)} />}
      <span className="truncate">{option.label}</span>
    </span>
  );
}

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
    // Boolean clauses use the operator ("is"/"is not") to express negation; value is always `true`.
    return true;
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
          className="h-7 w-32 shrink-0 text-xs"
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
        <SelectTrigger
          size="sm"
          className="h-7 w-24 shrink-0 text-xs"
          data-testid="filter-op-select"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {meta.ops.map((op) => (
            <SelectItem key={op} value={op} className="text-xs">
              {getOpLabel(op, meta.valueKind)}
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
  options: ValueOption[];
  onChange: (v: FilterValue) => void;
}) {
  const meta = getDimensionMeta(clause.dimension);

  if (meta.valueKind === "boolean") {
    // Value is implicit (always `true`); operator ("is"/"is not") expresses the predicate.
    return null;
  }

  if (meta.valueKind === "text") {
    return (
      <Input
        value={String(clause.value ?? "")}
        onChange={(e) => onChange(e.target.value)}
        placeholder={meta.placeholder ?? "Value"}
        className="h-7 min-w-0 flex-1 text-xs"
        data-testid="filter-value-input"
      />
    );
  }

  const multi = clause.op === "in" || clause.op === "not_in";

  if (multi) {
    const selected = Array.isArray(clause.value) ? clause.value.map(String) : [];
    return (
      <FilterMultiSelect options={options} selected={selected} onChange={(v) => onChange(v)} />
    );
  }

  const current = String(clause.value ?? "");
  return (
    <Select value={current} onValueChange={(v) => onChange(v)}>
      <SelectTrigger
        size="sm"
        className="h-7 min-w-0 flex-1 text-xs"
        data-testid="filter-value-select"
      >
        <SelectValue placeholder="Select value" />
      </SelectTrigger>
      <SelectContent>
        {options.length === 0 ? (
          <SelectItem value="__empty__" disabled className="text-xs">
            No options
          </SelectItem>
        ) : (
          <GroupedSelectItems options={options} />
        )}
      </SelectContent>
    </Select>
  );
}

function GroupedSelectItems({ options }: { options: ValueOption[] }) {
  if (!hasGroupedOptions(options)) {
    return (
      <>
        {options.map((opt) => (
          <SelectItem key={opt.value} value={opt.value} className="text-xs">
            <OptionLabel option={opt} />
          </SelectItem>
        ))}
      </>
    );
  }

  const groups = buildOptionGroups(options);

  return (
    <>
      {groups.map((g, idx) => (
        <Fragment key={g.heading || `__ungrouped__${idx}`}>
          {idx > 0 && <SelectSeparator />}
          <SelectGroup>
            {g.heading && <SelectLabel>{g.heading}</SelectLabel>}
            {g.items.map((opt) => (
              <SelectItem key={opt.value} value={opt.value} className="text-xs">
                <OptionLabel option={opt} />
              </SelectItem>
            ))}
          </SelectGroup>
        </Fragment>
      ))}
    </>
  );
}
