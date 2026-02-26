/** A panel within a group (tab). */
export type LayoutPanel = {
  id: string;
  component: string;
  title: string;
  tabComponent?: string;
  params?: Record<string, unknown>;
};

/** A group within a column (vertical slice). Contains tabs. */
export type LayoutGroup = {
  id?: string;
  panels: LayoutPanel[];
  activePanel?: string;
};

/** A leaf node in the column tree — contains a panel group. */
export type LayoutLeafNode = {
  type: "leaf";
  group: LayoutGroup;
  size?: number;
};

/** A branch node in the column tree — splits into child nodes. */
export type LayoutBranchNode = {
  type: "branch";
  children: LayoutNode[];
  size?: number;
};

/** A recursive node within a column's internal layout tree.
 *  Orientation alternates at each depth (VERTICAL at depth 0, HORIZONTAL at depth 1, etc.)
 *  matching dockview's grid structure. */
export type LayoutNode = LayoutLeafNode | LayoutBranchNode;

/** A column in the layout (horizontal slice). */
export type LayoutColumn = {
  id: string;
  pinned?: boolean;
  width?: number;
  maxWidth?: number;
  minWidth?: number;
  groups: LayoutGroup[];
  /** Optional nested tree structure capturing complex splits within the column.
   *  When present, takes precedence over `groups` for serialization. */
  tree?: LayoutNode;
};

/** Complete declarative layout state. */
export type LayoutState = {
  columns: LayoutColumn[];
};

// ─── Layout Intent ──────────────────────────────────────────────────────────

/** A panel to inject into the layout as part of an intent. */
export type LayoutIntentPanel = {
  id: string;
  component: string;
  title: string;
  tabComponent?: string;
  params?: Record<string, unknown>;
  /** Which group to add this panel to. Accepts a group ID (e.g. "group-center")
   *  or an alias: "center", "right-top", "right-bottom". Defaults to "center". */
  targetGroup?: string;
};

/**
 * Declarative description of the desired initial layout for a new session.
 * Set before navigation, consumed by buildDefaultLayout on first render.
 */
export type LayoutIntent = {
  /** Base preset to use. If omitted, uses userDefaultLayout or "default". */
  preset?: string;
  /** Additional panels to add after the base layout is applied. */
  panels?: LayoutIntentPanel[];
  /** Map of group ID → panel ID to set as the active tab in that group. */
  activePanels?: Record<string, string>;
};
