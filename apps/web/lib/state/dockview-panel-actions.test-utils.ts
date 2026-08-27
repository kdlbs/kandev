import type { AddPanelOptions, DockviewApi } from "dockview-react";
import { buildPanelActions } from "./dockview-panel-actions";
import type { ScrollTarget } from "./dockview-panel-actions";
import { CENTER_GROUP } from "./layout-manager";

export type MockPanel = {
  id: string;
  title: string;
  params: Record<string, unknown>;
  group: { id: string };
  isActive: boolean;
  api: {
    component: string;
    setActive: () => void;
    updateParameters: (p: Record<string, unknown>) => void;
    moveTo: (opts: { group: { id: string } }) => void;
  };
  setTitle: (t: string) => void;
};

/**
 * Build a minimal `DockviewApi` mock backed by in-memory panel/group arrays,
 * exposing `getPanel`/`getGroup`/`addPanel`/`removePanel` plus the active
 * panel and groups/panels getters.
 */
export function makeApi(
  options: { centerGroupId?: string; extraGroupIds?: string[] } = {},
): DockviewApi {
  const centerId = options.centerGroupId ?? CENTER_GROUP;
  const panels: MockPanel[] = [];
  const groups = [{ id: centerId }, ...(options.extraGroupIds ?? []).map((id) => ({ id }))];

  /**
   * Create a MockPanel from add options, registering the target group and
   * wiring the panel/API helpers (setTitle, setActive, updateParameters,
   * moveTo).
   */
  function makePanel(add: AddPanelOptions & { id: string }): MockPanel {
    const groupId =
      (add.position as { referenceGroup?: string } | undefined)?.referenceGroup ?? centerId;
    if (!groups.some((g) => g.id === groupId)) groups.push({ id: groupId });
    const panel: MockPanel = {
      id: add.id,
      title: (add.title as string) ?? "",
      params: { ...(add.params ?? {}) },
      group: { id: groupId },
      isActive: false,
      /** Set the mock panel's title. */
      setTitle(t: string) {
        this.title = t;
      },
      api: {
        component: add.component,
        /** Deactivate every panel and mark this one active. */
        setActive() {
          for (const p of panels) p.isActive = false;
          panel.isActive = true;
        },
        /** Merge the given params into the mock panel's params. */
        updateParameters(p: Record<string, unknown>) {
          Object.assign(panel.params, p);
        },
        /** Move the mock panel into the given group. */
        moveTo({ group }: { group: { id: string } }) {
          panel.group = { id: group.id };
        },
      },
    };
    return panel;
  }

  const api = {
    get groups() {
      return groups;
    },
    get panels() {
      return panels;
    },
    /** Return the mock panel with the given id, if any. */
    getPanel(id: string) {
      return panels.find((p) => p.id === id);
    },
    /** Return the mock group with the given id, if any. */
    getGroup(id: string) {
      return groups.find((g) => g.id === id);
    },
    /** Create and register a mock panel, activating it unless `inactive` is set. */
    addPanel(opts: AddPanelOptions & { id: string }) {
      const p = makePanel(opts);
      panels.push(p);
      if (!opts.inactive) p.api.setActive();
      return p;
    },
    /** Remove the mock panel with the matching id, if present. */
    removePanel(panel: { id: string }) {
      const i = panels.findIndex((p) => p.id === panel.id);
      if (i >= 0) panels.splice(i, 1);
    },
    get activePanel() {
      return panels.find((p) => p.isActive);
    },
  } as unknown as DockviewApi;
  return api;
}

type StoreShape = {
  api: DockviewApi | null;
  centerGroupId: string;
  rightTopGroupId: string;
  rightBottomGroupId: string;
  selectedDiff: { path: string; content?: string } | null;
  scrollTarget: ScrollTarget | null;
};

/**
 * Build a minimal store-shaped object (get/set/state) seeded with the mock
 * api and default group ids, mirroring the subset of the dockview store the
 * panel actions read and write.
 */
export function makeStore(api: DockviewApi) {
  const state: StoreShape = {
    api,
    centerGroupId: CENTER_GROUP,
    rightTopGroupId: "group-right-top",
    rightBottomGroupId: "group-right-bottom",
    selectedDiff: null,
    scrollTarget: null,
  };
  return {
    get: () => state,
    set: (partial: Partial<StoreShape>) => Object.assign(state, partial),
    state,
  };
}

/**
 * Build the store shape and the panel actions object bound to it, returning
 * api/actions/store for tests.
 */
export function build(api: DockviewApi) {
  const store = makeStore(api);
  const actions = buildPanelActions(store.set, store.get);
  return { api, actions, store };
}

export const PATH_A = "src/a.ts";
export const PATH_B = "src/b.ts";
export const PATH_NESTED_B = "src/nested/b.ts";
export const SHARED_PATH = "README.md";
export const NAME_A = "a.ts";
export const NAME_B = "b.ts";
export const SHARED_NAME = "README.md";
export const PINNED_FILE_A_ID = "file:src/a.ts";
export const PREVIEW_FILE_ID = "preview:file-editor";
export const PREVIEW_DIFF_ID = "preview:file-diff";
export const PREVIEW_COMMIT_ID = "preview:commit-detail";
export const SHA_A = "abcdef1234567890";
export const SHA_B = "fedcba0987654321";
export const DIFF_FILE_PREFIX = "diff:file:";
export const FILE_PREFIX_ID = "file:";
export const COMMIT_PREFIX_ID = "commit:";
export const TYPE_FILE_EDITOR = "file-editor" as const;
