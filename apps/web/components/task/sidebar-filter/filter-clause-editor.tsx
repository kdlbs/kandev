"use client";

import { useTranslation } from "react-i18next";
import type { FilterClause, FilterOp } from "@/lib/state/slices/ui/sidebar-view-types";
import {
  DIMENSION_METAS,
  getDimensionEnumOptions,
  getDimensionMeta,
  getOpLabel,
} from "./filter-dimension-registry";
import { useFilterValueOptions } from "./use-filter-value-options";
import { TypedFilterClauseEditor } from "./typed-filter-clause-editor";

type Props = {
  clause: FilterClause;
  onChange: (next: FilterClause) => void;
  onRemove: () => void;
};

export function FilterClauseEditor({ clause, onChange, onRemove }: Props) {
  const { t } = useTranslation();
  const meta = getDimensionMeta(clause.dimension);
  const dynamicOptions = useFilterValueOptions(clause.dimension);
  const options = getDimensionEnumOptions(meta) ?? dynamicOptions;

  return (
    <TypedFilterClauseEditor
      clause={clause}
      dimensions={DIMENSION_METAS}
      getMeta={getDimensionMeta}
      getDimensionLabel={(dimension) => t(getDimensionMeta(dimension).labelKey)}
      getOpLabel={(op, valueKind) => getOpLabel(op as FilterOp, valueKind)}
      optionsForDimension={() => options}
      onChange={onChange}
      onRemove={onRemove}
    />
  );
}
