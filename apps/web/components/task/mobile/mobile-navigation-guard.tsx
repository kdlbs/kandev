"use client";

import { createContext, useCallback, useContext, useEffect, useRef, type ReactNode } from "react";

export type MobileNavigationAction = () => void | Promise<void>;
export type MobileNavigationRequest = (action: MobileNavigationAction) => void;

type MobileNavigationGuardContextValue = {
  register: (request: MobileNavigationRequest | null) => void;
  request: MobileNavigationRequest;
};

const MobileNavigationGuardContext = createContext<MobileNavigationGuardContextValue>({
  register: () => undefined,
  request: (action) => {
    void action();
  },
});

export function MobileNavigationGuardProvider({ children }: { children: ReactNode }) {
  const requestRef = useRef<MobileNavigationRequest | null>(null);
  const register = useCallback((request: MobileNavigationRequest | null) => {
    requestRef.current = request;
  }, []);
  const request = useCallback<MobileNavigationRequest>((action) => {
    const guard = requestRef.current;
    if (guard) {
      guard(action);
      return;
    }
    void action();
  }, []);

  return (
    <MobileNavigationGuardContext.Provider value={{ register, request }}>
      {children}
    </MobileNavigationGuardContext.Provider>
  );
}

export function useMobileNavigationGuard() {
  return useContext(MobileNavigationGuardContext).request;
}

export function useRegisterMobileNavigationGuard(request: MobileNavigationRequest) {
  const register = useContext(MobileNavigationGuardContext).register;
  useEffect(() => {
    register(request);
    return () => register(null);
  }, [register, request]);
}
