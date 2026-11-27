import { createStateStore, type StateStore, type Spec, type UIElement } from "@json-render/core";

export const HOST_ELEMENT_ID = Symbol();
export type HostElement = UIElement & { [HOST_ELEMENT_ID]?: string };
const retained = new WeakMap<object, Map<string, StateStore>>();
const MAX_REPORTS = 32;

export function getRichOutputViewStore(owner: object, identity: string): StateStore {
  let reports = retained.get(owner);
  if (!reports) {
    reports = new Map();
    retained.set(owner, reports);
  }
  const store = reports.get(identity) ?? createStateStore();
  reports.delete(identity);
  reports.set(identity, store);
  if (reports.size > MAX_REPORTS) reports.delete(reports.keys().next().value!);
  return store;
}

export function withHostElementIds(spec: Spec): Spec {
  return {
    ...spec,
    elements: Object.fromEntries(
      Object.entries(spec.elements).map(([id, element]) => [
        id,
        { ...element, [HOST_ELEMENT_ID]: id },
      ]),
    ),
  };
}
