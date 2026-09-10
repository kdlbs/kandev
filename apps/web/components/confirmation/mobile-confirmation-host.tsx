"use client";

import {
  createContext,
  useCallback,
  useContext,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from "react";

type Request = {
  token: symbol;
  titleId: string;
  descriptionId: string;
  cancel: (invalidated?: boolean) => void;
};

type HostContext = {
  request: Request | null;
  outlet: HTMLDivElement | null;
  setOutlet: (node: HTMLDivElement | null) => void;
  register: (request: Request) => void;
  release: (token: symbol) => void;
  originRef: RefObject<HTMLDivElement | null>;
  restoreFocus: (preferred: HTMLElement | null) => void;
};

const Context = createContext<HostContext | null>(null);
export const useMobileConfirmationHost = () => useContext(Context);

type HostState = {
  active: boolean;
  contentProps: {
    "aria-labelledby"?: string;
    "aria-describedby"?: string;
    onEscapeKeyDown?: (event: KeyboardEvent) => void;
  };
};

/** Owns one step, not another modal. The actual Drawer/Dialog receives contentProps. */
export function MobileConfirmationHost({
  open,
  children,
}: {
  open: boolean;
  children: (state: HostState) => ReactNode;
}) {
  const current = useRef<Request | null>(null);
  const originRef = useRef<HTMLDivElement>(null);
  const [request, setRequest] = useState<Request | null>(null);
  const [outlet, setOutlet] = useState<HTMLDivElement | null>(null);
  const restoreFocus = useCallback((preferred: HTMLElement | null) => {
    if (current.current || !originRef.current?.isConnected) return;
    if (preferred && originRef.current.contains(preferred)) {
      preferred.focus({ preventScroll: true });
      if (document.activeElement === preferred) return;
    }
    const controls = originRef.current.querySelectorAll<HTMLElement>(
      'button:not(:disabled), input:not(:disabled), select:not(:disabled), a[href], [tabindex="0"]',
    );
    const fallback = Array.from(controls).find(
      (control) => !control.closest('[hidden], [inert], [aria-hidden="true"]'),
    );
    fallback?.focus({ preventScroll: true });
  }, []);
  const release = useCallback((token: symbol) => {
    if (current.current?.token !== token) return;
    current.current = null;
    setRequest(null);
  }, []);
  const register = useCallback((next: Request) => {
    if (current.current?.token === next.token) return;
    const previous = current.current;
    current.current = next;
    previous?.cancel(true);
    setRequest(next);
  }, []);
  useLayoutEffect(() => {
    if (open || !current.current) return;
    const previous = current.current;
    release(previous.token);
    previous.cancel(true);
  }, [open, request, release]);
  const context = useMemo(
    () => ({ request, outlet, setOutlet, register, release, originRef, restoreFocus }),
    [request, outlet, register, release, restoreFocus],
  );
  const contentProps = request
    ? {
        "aria-labelledby": request.titleId,
        "aria-describedby": request.descriptionId,
        onEscapeKeyDown: (event: KeyboardEvent) => {
          event.preventDefault();
          event.stopPropagation();
          request.cancel();
        },
      }
    : {};
  return (
    <Context.Provider value={context}>
      {children({ active: request !== null, contentProps })}
    </Context.Provider>
  );
}

/** Visibility preserves layout and scroll; the sibling portal is never hidden with its source. */
export function MobileConfirmationHostBody({ children }: { children: ReactNode }) {
  const host = useMobileConfirmationHost();
  if (!host) return children;
  const active = host.request !== null;
  return (
    <div className="grid min-h-0 min-w-0 flex-1 grid-cols-1 grid-rows-1">
      <div
        ref={host.originRef}
        aria-hidden={active || undefined}
        inert={active || undefined}
        style={active ? { visibility: "hidden" } : undefined}
        className="col-start-1 row-start-1 flex min-h-0 min-w-0 flex-col"
      >
        {children}
      </div>
      <div
        ref={host.setOutlet}
        className="col-start-1 row-start-1 flex min-h-0 min-w-0 flex-col"
        hidden={!active}
      />
    </div>
  );
}
