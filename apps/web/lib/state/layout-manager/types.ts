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
