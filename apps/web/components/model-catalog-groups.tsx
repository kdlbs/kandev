"use client";

import { Fragment, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconCheck, IconChevronRight } from "@tabler/icons-react";

import * as selectorOptions from "@/lib/utils/selector-options";
import type {
  CatalogEntry,
  CatalogGroupEntry,
  CatalogLeafEntry,
  CatalogTree,
} from "@/lib/model-catalog-selection";
import type { ModelCatalogComposition } from "@kandev/types/model-catalog";
import { ModelRow } from "@/components/model-config-selector-rows";
import { CommandGroup, CommandItem } from "@kandev/ui/command";
import { Spinner } from "@kandev/ui/spinner";

type RenderContext = {
  currentModel: string;
  loading: boolean;
  onSelectModel: (value: string) => void;
  onOpenComposition: (group: CatalogGroupEntry) => void;
  compositions: ModelCatalogComposition[] | undefined;
};

function leafModel(leaf: CatalogLeafEntry) {
  const pathLabel = leaf.path.map((part) => part.label).join(" / ");
  // Path labels stay in the filter keywords even when the provider supplies a
  // description, so a leaf remains discoverable by its group.
  return {
    id: leaf.value,
    name: leaf.label,
    ...(leaf.description ? { description: leaf.description } : {}),
    ...(pathLabel ? { keywords: [pathLabel] } : {}),
  };
}

function CatalogLeafRow({ leaf, context }: { leaf: CatalogLeafEntry; context: RenderContext }) {
  return (
    <ModelRow
      model={leafModel(leaf)}
      selected={leaf.value === context.currentModel}
      loading={context.loading}
      onSelect={context.onSelectModel}
    />
  );
}

function compositionKeywords(
  composition: ModelCatalogComposition,
  group: CatalogGroupEntry,
): string[] {
  return [
    group.label,
    composition.label ?? "",
    composition.id,
    ...composition.dimensions.flatMap((dimension) => [
      dimension.label,
      ...dimension.choices.flatMap((choice) => [choice.label, choice.value]),
    ]),
    ...composition.bindings.map((binding) => binding.model),
  ];
}

function CompositionRow({
  group,
  composition,
  context,
}: {
  group: CatalogGroupEntry;
  composition: ModelCatalogComposition;
  context: RenderContext;
}) {
  const { t } = useTranslation();
  const selected = composition.bindings.some((binding) => binding.model === context.currentModel);
  return (
    <CommandItem
      value={`composition:${group.id}`}
      keywords={compositionKeywords(composition, group)}
      onSelect={() => context.onOpenComposition(group)}
      aria-label={t("agents:configureModelGroup", { name: group.label })}
      data-testid={`model-catalog-composition-row-${group.id}`}
      className={selectorOptions.selectorOptionClassName(selected, false)}
    >
      <div className="flex min-w-0 flex-1 items-center">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate">{group.label}</span>
            <span className="shrink-0 text-xs text-muted-foreground">
              {t("agents:modelConfigurationDimensions", { count: composition.dimensions.length })}
            </span>
          </div>
          {group.description && (
            <div className="truncate text-xs text-muted-foreground">{group.description}</div>
          )}
        </div>
        {selected && context.loading ? (
          <Spinner aria-hidden="true" className="absolute right-8" />
        ) : (
          <IconCheck
            className={
              selected
                ? "absolute right-8 h-4 w-4 opacity-100"
                : "absolute right-8 h-4 w-4 opacity-0"
            }
          />
        )}
        <IconChevronRight className="ml-auto h-3.5 w-3.5 shrink-0 text-muted-foreground" />
      </div>
    </CommandItem>
  );
}

function renderEntries(
  entries: CatalogEntry[],
  context: RenderContext,
  depth: number,
): ReactNode[] {
  return entries.map((entry) => {
    if (entry.kind === "choice") {
      return <CatalogLeafRow key={entry.key} leaf={entry} context={context} />;
    }
    const composition = context.compositions?.find((candidate) => candidate.group_id === entry.id);
    if (composition) {
      const bound = new Set(composition.bindings.map((binding) => binding.model));
      const unbound = entry.children.filter(
        (child): child is CatalogLeafEntry => child.kind === "choice" && !bound.has(child.value),
      );
      return (
        <Fragment key={entry.key}>
          <CompositionRow group={entry} composition={composition} context={context} />
          {unbound.map((leaf) => (
            <CatalogLeafRow key={leaf.key} leaf={leaf} context={context} />
          ))}
        </Fragment>
      );
    }
    return (
      <CommandGroup
        key={entry.key}
        heading={<span className={depth > 0 ? "pl-3" : undefined}>{entry.label}</span>}
      >
        {renderEntries(entry.children, context, depth + 1)}
      </CommandGroup>
    );
  });
}

export type CatalogCommandGroupsProps = RenderContext & {
  tree: CatalogTree;
  heading: ReactNode;
};

/**
 * Renders the provider's catalog structure inside the existing command list.
 * Ordinary groups keep their native order; a group with verified bindings opens
 * the focused composition view instead of listing exploded pairings.
 */
export function CatalogCommandGroups({ tree, heading, ...context }: CatalogCommandGroupsProps) {
  const looseLeaves = tree.entries.filter(
    (entry): entry is CatalogLeafEntry => entry.kind === "choice",
  );
  const grouped = tree.entries.filter((entry) => entry.kind === "group");
  return (
    <>
      {(looseLeaves.length > 0 || grouped.length === 0) && (
        <CommandGroup heading={heading}>
          {looseLeaves.map((leaf) => (
            <CatalogLeafRow key={leaf.key} leaf={leaf} context={context} />
          ))}
        </CommandGroup>
      )}
      {renderEntries(grouped, context, 0)}
    </>
  );
}
