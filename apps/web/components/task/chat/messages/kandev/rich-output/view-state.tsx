import { createContext, useCallback, useContext, useState, useSyncExternalStore } from "react";
import { createStateStore, type StateStore, type ComponentRenderer } from "@json-render/react";
import { HOST_ELEMENT_ID, type HostElement } from "@/lib/rich-output/view-state";

export const RichOutputViewContext = createContext<StateStore | undefined>(undefined);
const ElementContext = createContext<string | undefined>(undefined);

export function withElementViewState(Component: ComponentRenderer): ComponentRenderer {
  return function ElementViewState(props) {
    return (
      <ElementContext.Provider value={(props.element as HostElement)[HOST_ELEMENT_ID]}>
        <Component {...props} />
      </ElementContext.Provider>
    );
  };
}

export function useRichOutputViewState<T>(field: string, initial: T) {
  const hostStore = useContext(RichOutputViewContext);
  const elementId = useContext(ElementContext);
  const [local] = useState(() => createStateStore({ value: initial }));
  const store = hostStore ?? local;
  const path = `/${JSON.stringify([elementId, field]).replaceAll("~", "~0").replaceAll("/", "~1")}`;
  const getSnapshot = useCallback(
    () => (store.get(path) ?? local.get("/value")) as T,
    [store, path, local],
  );
  const value = useSyncExternalStore(store.subscribe, getSnapshot, getSnapshot);
  const set = useCallback(
    (next: T | ((previous: T) => T)) => {
      const updated =
        typeof next === "function" ? (next as (previous: T) => T)(getSnapshot()) : next;
      store.set(path, updated);
    },
    [getSnapshot, path, store],
  );
  return [value, set] as const;
}
