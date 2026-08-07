"use client";

import { useTranslation } from "react-i18next";

import { AgentLogo } from "@/components/agent-logo";
import { ActiveWorkspaceBadge, DisabledBadge } from "@/components/settings/record-badges";
import type { SettingsMenuNode } from "./settings-menu-branches";
import { RecordGlyph, SettingsBranch, SettingsLeaf } from "./settings-nav-primitives";
import type { SettingsMenuExpansion } from "./use-settings-menu-expansion";

type SettingsMenuNodeRowProps = {
  node: SettingsMenuNode;
  /** Nesting level. Menu rows are 0, so a branch's own children start at 1. */
  depth: number;
  /**
   * The single node allowed to mark itself active — the deepest one owning the
   * current route. Ancestors merely hold it open.
   */
  activeKey: string | null;
  expansion: SettingsMenuExpansion;
};

/**
 * One row of a settings menu branch, recursing into its children.
 *
 * A node with children renders as a disclosure; a node without renders as a
 * plain link. A node with neither children nor an `href` renders nothing at
 * all — it could not be navigated to or opened, so it would be a dead row.
 */
/**
 * The row's leading visual: an agent's logo, the record dot, or nothing — in
 * which case `NavRowLabel` keeps the glyph box open. A node that already
 * declares an `icon` is left to it.
 */
function resolveLeadingIcon(node: SettingsMenuNode, isRecord: boolean) {
  if (node.agentName) {
    return <AgentLogo agentName={node.agentName} className="h-3.5 w-3.5 shrink-0" />;
  }
  if (isRecord && !node.icon) return <RecordGlyph />;
  return undefined;
}

/** One badge per node at most; `undefined` is much the commonest case. */
function renderBadge(badge: SettingsMenuNode["badge"]) {
  if (badge === "disabled") return <DisabledBadge />;
  if (badge === "active") return <ActiveWorkspaceBadge />;
  return undefined;
}

export function SettingsMenuNodeRow({
  node,
  depth,
  activeKey,
  expansion,
}: SettingsMenuNodeRowProps) {
  const { t } = useTranslation();
  const label = "key" in node.label ? t(node.label.key) : node.label.text;
  const isRecord = node.isUserRecord === true;
  // Records carry no glyph of their own, so the dot stands in as one. Supplied
  // here rather than by the branch builders: it belongs to being a record, and
  // stating it once beats repeating it in all three of them.
  const leadingIcon = resolveLeadingIcon(node, isRecord);
  const isActive = node.key === activeKey;
  const children = node.children ?? [];
  const labelSuffix = renderBadge(node.badge);

  if (children.length === 0) {
    if (!node.href) return null;
    return (
      <SettingsLeaf
        href={node.href}
        label={label}
        icon={node.icon}
        leadingIcon={leadingIcon}
        labelSuffix={labelSuffix}
        isActive={isActive}
        depth={depth}
        isRecord={isRecord}
      />
    );
  }

  return (
    <SettingsBranch
      label={label}
      icon={node.icon}
      leadingIcon={leadingIcon}
      labelSuffix={labelSuffix}
      href={node.href ?? undefined}
      isActive={isActive}
      expanded={expansion.isExpanded(node.key)}
      onToggle={() => expansion.toggle(node.key)}
      depth={depth}
      isRecord={isRecord}
    >
      {children.map((child) => (
        <SettingsMenuNodeRow
          key={child.key}
          node={child}
          depth={depth + 1}
          activeKey={activeKey}
          expansion={expansion}
        />
      ))}
    </SettingsBranch>
  );
}
