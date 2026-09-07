"use client";

import { useTranslation } from "react-i18next";
import type { ThreadCandidate } from "@/lib/threads/thread-view-query";
import type { ThreadFilterClause } from "@/lib/state/slices/ui/thread-view-types";
import { TypedFilterClauseEditor } from "@/components/task/sidebar-filter/typed-filter-clause-editor";
import {
  getThreadDimensionLabel,
  getThreadDimensionMeta,
  getThreadFilterOpLabel,
  getThreadFilterOptions,
  THREAD_DIMENSION_METAS,
} from "./threads-view-filter-registry";

export function ThreadsViewFilterRow({
  clause,
  candidates,
  repositoryNames,
  mobile,
  onChange,
  onRemove,
}: {
  clause: ThreadFilterClause;
  candidates: ThreadCandidate[];
  repositoryNames: ReadonlyMap<string, string>;
  mobile?: boolean;
  onChange: (next: ThreadFilterClause) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  return (
    <TypedFilterClauseEditor
      clause={clause}
      dimensions={THREAD_DIMENSION_METAS}
      getMeta={getThreadDimensionMeta}
      getDimensionLabel={(dimension) => getThreadDimensionLabel(dimension, t)}
      getOpLabel={(op) => getThreadFilterOpLabel(op, t)}
      optionsForDimension={(dimension) =>
        getThreadFilterOptions(dimension, candidates, t, repositoryNames)
      }
      onChange={onChange}
      onRemove={onRemove}
      mobile={mobile}
      testIds={{
        row: "threads-filter-row",
        dimension: "threads-filter-dimension",
        op: "threads-filter-op",
        value: "threads-filter-value",
        textValue: "threads-filter-value",
        remove: "threads-filter-remove",
      }}
    />
  );
}
