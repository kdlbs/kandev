import { createDockview, type DockviewApi } from "dockview-react";

/**
 * A real Dockview instance laid out at a fixed size in jsdom. Layout geometry is
 * driven entirely by `api.layout(width, height)`, so sizes are deterministic.
 * Panel content and tabs are inert elements: tests assert on structure and
 * geometry, never on what a panel renders.
 */
export function mountDockview(width: number, height: number): DockviewApi {
  const element = document.createElement("div");
  document.body.appendChild(element);
  const inert = () => ({ element: document.createElement("div"), init() {}, dispose() {} });
  const api = createDockview(element, {
    createComponent: inert,
    createTabComponent: inert,
  } as never);
  api.layout(width, height);
  return api;
}

/** Remove a Dockview created by `mountDockview` together with its host element. */
export function disposeDockview(api: DockviewApi): void {
  const element = (api as unknown as { component?: { element?: HTMLElement } }).component?.element;
  api.dispose();
  element?.parentElement?.remove();
}
