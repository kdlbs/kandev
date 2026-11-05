"use client";

import { type ReactNode } from "react";
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

type ProjectedRow = {
  kind: "row";
  commandId: string;
  leaf: CatalogLeafEntry;
  detail?: string;
};

type ProjectedGroup = {
  kind: "group";
  key: string;
  group: CatalogGroupEntry;
  children: ProjectedEntry[];
};

type ProjectedComposition = {
  kind: "composition";
  commandId: string;
  group: CatalogGroupEntry;
  composition: ModelCatalogComposition;
  unbound: ProjectedRow[];
};

type ProjectedEntry = ProjectedRow | ProjectedGroup | ProjectedComposition;

type CommandIdentityKind = "row" | "composition";

type CommandIdentities = {
  allocate: (kind: CommandIdentityKind, base: string) => string;
};

/**
 * Allocates one unambiguous command identity per rendered entry. `base` may
 * repeat (provider values can repeat under different paths), so every
 * allocation carries a declared kind prefix and a counted suffix. cmdk selects
 * items by value; duplicate values would mark several entries selected and
 * dispatch Enter to the first one.
 */
function createCommandIdentities(): CommandIdentities {
  const counts = new Map<string, number>();
  return {
    allocate(kind, base) {
      const key = `${kind}:${base}`;
      const occurrence = counts.get(key) ?? 0;
      counts.set(key, occurrence + 1);
      return `${key}#${occurrence}`;
    },
  };
}

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

function CatalogLeafRow({
  leaf,
  commandId,
  detail,
  context,
}: {
  leaf: CatalogLeafEntry;
  commandId: string;
  detail?: string;
  context: RenderContext;
}) {
  return (
    <ModelRow
      model={leafModel(leaf)}
      itemValue={commandId}
      detail={detail}
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
  commandId,
  context,
}: {
  group: CatalogGroupEntry;
  composition: ModelCatalogComposition;
  commandId: string;
  context: RenderContext;
}) {
  const { t } = useTranslation();
  const selected = composition.bindings.some((binding) => binding.model === context.currentModel);
  return (
    <CommandItem
      value={commandId}
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

function verifiedComposition(
  compositions: ModelCatalogComposition[] | undefined,
  groupID: string,
): ModelCatalogComposition | undefined {
  const composition = compositions?.find((candidate) => candidate.group_id === groupID);
  return composition && composition.bindings.length > 0 ? composition : undefined;
}

function subtreeHasComposition(
  entry: CatalogGroupEntry,
  compositions: ModelCatalogComposition[] | undefined,
): boolean {
  if (verifiedComposition(compositions, entry.id)) return true;
  return entry.children.some(
    (child) => child.kind === "group" && subtreeHasComposition(child, compositions),
  );
}

type SingletonCollapse = { leaf: CatalogLeafEntry; removed: string[] };

function descendSingleton(
  entry: CatalogGroupEntry,
  labels: string[],
): SingletonCollapse | undefined {
  const chain = [...labels, entry.label];
  if (entry.children.length !== 1) return undefined;
  const next = entry.children[0];
  if (next.kind === "group") return descendSingleton(next, chain);
  return { leaf: next, removed: chain };
}

/**
 * A singleton chain collapses only when its whole subtree holds exactly one
 * ordinary choice and no composition. The check runs on the full catalog, so a
 * search that leaves one result never changes the presentation.
 */
function collapseSingleton(
  entry: CatalogGroupEntry,
  compositions: ModelCatalogComposition[] | undefined,
): SingletonCollapse | undefined {
  if (entry.leafCount !== 1 || subtreeHasComposition(entry, compositions)) return undefined;
  return descendSingleton(entry, []);
}

function removedPathDetail(collapse: SingletonCollapse): string | undefined {
  const labels: string[] = [];
  for (const label of collapse.removed) {
    if (label === collapse.leaf.label) continue;
    if (labels[labels.length - 1] === label) continue;
    labels.push(label);
  }
  return labels.length > 0 ? labels.join(" / ") : undefined;
}

function singletonRow(
  entry: CatalogGroupEntry,
  compositions: ModelCatalogComposition[] | undefined,
  identities: CommandIdentities,
): ProjectedRow | undefined {
  const collapse = collapseSingleton(entry, compositions);
  if (!collapse) return undefined;
  const detail = removedPathDetail(collapse);
  return {
    kind: "row",
    commandId: identities.allocate("row", collapse.leaf.key),
    leaf: collapse.leaf,
    ...(detail ? { detail } : {}),
  };
}

function projectGroup(
  entry: CatalogGroupEntry,
  compositions: ModelCatalogComposition[] | undefined,
  identities: CommandIdentities,
): ProjectedEntry {
  const composition = verifiedComposition(compositions, entry.id);
  if (composition) {
    const bound = new Set(composition.bindings.map((binding) => binding.model));
    const unbound = entry.children
      .filter((child): child is CatalogLeafEntry => child.kind === "choice")
      .filter((child) => !bound.has(child.value))
      .map((leaf) => ({
        kind: "row" as const,
        commandId: identities.allocate("row", leaf.key),
        leaf,
      }));
    return {
      kind: "composition",
      commandId: identities.allocate("composition", entry.id),
      group: entry,
      composition,
      unbound,
    };
  }
  const row = singletonRow(entry, compositions, identities);
  if (row) return row;
  return {
    kind: "group",
    key: entry.key,
    group: entry,
    children: projectEntries(entry.children, compositions, identities),
  };
}

function projectEntries(
  entries: CatalogEntry[],
  compositions: ModelCatalogComposition[] | undefined,
  identities: CommandIdentities,
): ProjectedEntry[] {
  return entries.map((entry) =>
    entry.kind === "choice"
      ? { kind: "row", commandId: identities.allocate("row", entry.key), leaf: entry }
      : projectGroup(entry, compositions, identities),
  );
}

function rowNodes(rows: ProjectedRow[], context: RenderContext): ReactNode[] {
  return rows.map((row) => (
    <CatalogLeafRow
      key={row.commandId}
      leaf={row.leaf}
      commandId={row.commandId}
      detail={row.detail}
      context={context}
    />
  ));
}

function ProjectedGroupNode({
  entry,
  context,
  depth,
}: {
  entry: ProjectedGroup;
  context: RenderContext;
  depth: number;
}) {
  // cmdk registers an item under its nearest group only. A group whose children
  // are all nested groups has no direct item, so it would be hidden during a
  // search that matches a nested choice and take that nested heading with it.
  const nestedOnly =
    entry.children.length > 0 && entry.children.every((child) => child.kind === "group");
  return (
    <CommandGroup
      forceMount={nestedOnly}
      heading={<span className={depth > 0 ? "pl-3" : undefined}>{entry.group.label}</span>}
    >
      {entry.children.map((child) => renderChild(child, context, depth + 1))}
    </CommandGroup>
  );
}

function ProjectedCompositionNode({
  entry,
  context,
}: {
  entry: ProjectedComposition;
  context: RenderContext;
}) {
  return (
    <>
      <CompositionRow
        group={entry.group}
        composition={entry.composition}
        commandId={entry.commandId}
        context={context}
      />
      {rowNodes(entry.unbound, context)}
    </>
  );
}

function renderChild(entry: ProjectedEntry, context: RenderContext, depth: number): ReactNode {
  if (entry.kind === "row") {
    return (
      <CatalogLeafRow
        key={entry.commandId}
        leaf={entry.leaf}
        commandId={entry.commandId}
        detail={entry.detail}
        context={context}
      />
    );
  }
  return entry.kind === "group" ? (
    <ProjectedGroupNode key={entry.key} entry={entry} context={context} depth={depth} />
  ) : (
    <ProjectedCompositionNode key={entry.commandId} entry={entry} context={context} />
  );
}

function renderEntries(
  entries: ProjectedEntry[],
  context: RenderContext,
  depth: number,
  heading?: ReactNode,
): ReactNode[] {
  const nodes: ReactNode[] = [];
  let pendingRows: ProjectedRow[] = [];
  let headingPending = heading !== undefined;
  const flushRows = () => {
    if (pendingRows.length === 0) return;
    nodes.push(
      <CommandGroup
        key={`rows:${pendingRows[0].commandId}`}
        heading={headingPending ? heading : undefined}
      >
        {rowNodes(pendingRows, context)}
      </CommandGroup>,
    );
    headingPending = false;
    pendingRows = [];
  };
  for (const entry of entries) {
    if (entry.kind === "row") {
      pendingRows.push(entry);
      continue;
    }
    flushRows();
    nodes.push(renderChild(entry, context, depth));
  }
  flushRows();
  return nodes;
}

export type CatalogCommandGroupsProps = RenderContext & {
  tree: CatalogTree;
  heading: ReactNode;
};

/**
 * Renders the provider's catalog structure inside the existing command list.
 * Ordinary singleton chains project into one row, multi-choice groups keep
 * their heading, and a verified composition opens the focused dimensions view.
 * Entries render in one ordered traversal, so loose choices are never hoisted.
 */
export function CatalogCommandGroups({ tree, heading, ...context }: CatalogCommandGroupsProps) {
  const identities = createCommandIdentities();
  const projected = projectEntries(tree.entries, context.compositions, identities);
  return <>{renderEntries(projected, context, 0, heading)}</>;
}
