import { renderHook } from "@testing-library/react";
import { useState, type ReactNode } from "react";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";

export function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

export function renderSessionRead<T, P>(
  read: (props: P) => T,
  initialProps: P,
  setup?: (store: StoreApi<AppState>) => void,
) {
  function Initialize({ children }: { children: ReactNode }) {
    const store = useAppStoreApi();
    useState(() => {
      store.setState({
        environmentIdBySessionId: { session: "environment", other: "other-environment" },
      });
      setup?.(store);
      return true;
    });
    return children;
  }
  return renderHook((props: P) => ({ store: useAppStoreApi(), value: read(props) }), {
    initialProps,
    wrapper: ({ children }) => (
      <StateProvider
        initialState={{
          connection: { status: "connected", error: null, issueSeverity: "none" },
        }}
      >
        <Initialize>{children}</Initialize>
      </StateProvider>
    ),
  });
}
