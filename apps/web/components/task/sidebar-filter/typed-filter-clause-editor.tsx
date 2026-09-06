"use client";

import { Fragment } from "react";
import { IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
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
import { cn } from "@/lib/utils";
import { FilterMultiSelect, type MultiSelectOption } from "./filter-multi-select";
import { buildOptionGroups, hasGroupedOptions } from "./filter-option-groups";

export type TypedFilterValue = string | string[] | boolean;

export type TypedFilterClause<Dimension extends string, Op extends string> = {
  id: string;
  dimension: Dimension;
  op: Op;
  value: TypedFilterValue;
};

export type TypedFilterMeta<Dimension extends string, Op extends string> = {
  dimension: Dimension;
  valueKind: "boolean" | "enum" | "text";
  ops: readonly Op[];
  defaultOp: Op;
  defaultValue: TypedFilterValue;
  placeholder?: string;
};

type ValueOption = MultiSelectOption;

type TestIds = {
  row: string;
  dimension: string;
  op: string;
  value: string;
  textValue?: string;
  remove: string;
};

type Props<Dimension extends string, Op extends string> = {
  clause: TypedFilterClause<Dimension, Op>;
  dimensions: readonly TypedFilterMeta<Dimension, Op>[];
  getMeta: (dimension: Dimension) => TypedFilterMeta<Dimension, Op>;
  getDimensionLabel: (dimension: Dimension) => string;
  getOpLabel: (op: Op, valueKind: TypedFilterMeta<Dimension, Op>["valueKind"]) => string;
  optionsForDimension: (dimension: Dimension) => ValueOption[];
  onChange: (next: TypedFilterClause<Dimension, Op>) => void;
  onRemove: () => void;
  mobile?: boolean;
  testIds?: Partial<TestIds>;
};

const DEFAULT_TEST_IDS: TestIds = {
  row: "filter-clause-row",
  dimension: "filter-dimension-select",
  op: "filter-op-select",
  value: "filter-value-select",
  textValue: "filter-value-input",
  remove: "filter-clause-remove",
};

export function TypedFilterClauseEditor<Dimension extends string, Op extends string>({
  clause,
  dimensions,
  getMeta,
  getDimensionLabel,
  getOpLabel,
  optionsForDimension,
  onChange,
  onRemove,
  mobile = false,
  testIds: customTestIds,
}: Props<Dimension, Op>) {
  const { t } = useTranslation();
  const testIds = { ...DEFAULT_TEST_IDS, ...customTestIds };
  const meta = getMeta(clause.dimension);
  const options = optionsForDimension(clause.dimension);

  function changeDimension(dimension: Dimension) {
    const nextMeta = getMeta(dimension);
    onChange({
      ...clause,
      dimension,
      op: nextMeta.defaultOp,
      value: nextMeta.defaultValue,
    });
  }

  function changeOp(op: Op) {
    onChange({ ...clause, op, value: normalizeFilterValue(clause.value, meta, op) });
  }

  return (
    <div
      className={cn(
        "flex items-center gap-1.5 py-1",
        mobile && "[&_button]:min-h-11 [&_input]:min-h-11",
      )}
      data-testid={testIds.row}
      data-clause-id={clause.id}
    >
      <Select
        value={clause.dimension}
        onValueChange={(value) => changeDimension(value as Dimension)}
      >
        <SelectTrigger
          size="sm"
          className="h-7 w-32 shrink-0 text-xs"
          data-testid={testIds.dimension}
        >
          <SelectValue>{getDimensionLabel(clause.dimension)}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          {dimensions.map((dimension) => (
            <SelectItem key={dimension.dimension} value={dimension.dimension} className="text-xs">
              {getDimensionLabel(dimension.dimension)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select value={clause.op} onValueChange={(value) => changeOp(value as Op)}>
        <SelectTrigger size="sm" className="h-7 w-24 shrink-0 text-xs" data-testid={testIds.op}>
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

      <FilterValueInput
        clause={clause}
        meta={meta}
        options={options}
        valueTestId={
          meta.valueKind === "text" ? (testIds.textValue ?? testIds.value) : testIds.value
        }
        mobile={mobile}
        onChange={(value) => onChange({ ...clause, value })}
        valuePlaceholder={meta.placeholder ?? t("task:value")}
        selectValuePlaceholder={t("task:selectValue")}
        noOptionsLabel={t("task:noOptions")}
      />

      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="h-6 w-6 shrink-0 cursor-pointer text-muted-foreground hover:text-foreground"
        onClick={onRemove}
        data-testid={testIds.remove}
        aria-label={t("task:removeFilter")}
      >
        <IconX className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

function normalizeFilterValue<Dimension extends string, Op extends string>(
  value: TypedFilterValue,
  meta: TypedFilterMeta<Dimension, Op>,
  op: Op,
): TypedFilterValue {
  if (meta.valueKind === "boolean") return true;
  if (meta.valueKind === "enum" && (op === "in" || op === "not_in")) {
    if (Array.isArray(value)) return value;
    if (value) return [String(value)];
    return [];
  }
  return Array.isArray(value) ? (value[0] ?? "") : String(value);
}

function FilterValueInput<Dimension extends string, Op extends string>({
  clause,
  meta,
  options,
  valueTestId,
  mobile,
  onChange,
  valuePlaceholder,
  selectValuePlaceholder,
  noOptionsLabel,
}: {
  clause: TypedFilterClause<Dimension, Op>;
  meta: TypedFilterMeta<Dimension, Op>;
  options: ValueOption[];
  valueTestId: string;
  mobile: boolean;
  onChange: (value: TypedFilterValue) => void;
  valuePlaceholder: string;
  selectValuePlaceholder: string;
  noOptionsLabel: string;
}) {
  if (meta.valueKind === "boolean") return null;

  if (meta.valueKind === "text") {
    return (
      <Input
        value={String(clause.value ?? "")}
        onChange={(event) => onChange(event.target.value)}
        placeholder={valuePlaceholder}
        className={cn("h-7 min-w-0 flex-1 text-xs", mobile && "h-11")}
        data-testid={valueTestId}
      />
    );
  }

  const multi = clause.op === "in" || clause.op === "not_in";
  if (multi) {
    const selected = Array.isArray(clause.value) ? clause.value.map(String) : [];
    return (
      <FilterMultiSelect
        options={options}
        selected={selected}
        onChange={onChange}
        className={mobile ? "h-11" : undefined}
      />
    );
  }

  const current = String(clause.value ?? "");
  return (
    <Select value={current} onValueChange={onChange}>
      <SelectTrigger size="sm" className="h-7 min-w-0 flex-1 text-xs" data-testid={valueTestId}>
        <SelectValue placeholder={selectValuePlaceholder} />
      </SelectTrigger>
      <SelectContent>
        {options.length === 0 ? (
          <SelectItem value="__empty__" disabled className="text-xs">
            {noOptionsLabel}
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
    return options.map((option) => <SelectOption key={option.value} option={option} />);
  }

  return buildOptionGroups(options).map((group, index) => (
    <Fragment key={group.heading || `__ungrouped__${index}`}>
      {index > 0 && <SelectSeparator />}
      <SelectGroup>
        {group.heading && <SelectLabel>{group.heading}</SelectLabel>}
        {group.items.map((option) => (
          <SelectOption key={option.value} option={option} />
        ))}
      </SelectGroup>
    </Fragment>
  ));
}

function SelectOption({ option }: { option: ValueOption }) {
  return (
    <SelectItem value={option.value} className="text-xs">
      <span className="flex items-center gap-1.5">
        {option.color && (
          <span className={cn("block h-2 w-2 shrink-0 rounded-full", option.color)} />
        )}
        <span className="truncate">{option.label}</span>
      </span>
    </SelectItem>
  );
}
