"use client";

import { createContext, useContext, useId, useLayoutEffect, type ReactNode } from "react";
import { cn } from "./lib/utils";

export type ModalToastOutletContextValue = {
  activeOutletId: string | null;
  content: ReactNode;
  announcement: string;
  registerOutlet: (id: string) => () => void;
};

export const ModalToastOutletContext = createContext<ModalToastOutletContextValue | null>(null);

export function ToastAnnouncementRegion({ announcement }: { announcement: string }) {
  return (
    <div
      data-slot="toast-announcer"
      data-testid="toast-announcer"
      aria-live="polite"
      aria-atomic="true"
      className="sr-only"
    >
      {announcement}
    </div>
  );
}

export function ModalToastOutlet({ className }: { className?: string }) {
  const context = useContext(ModalToastOutletContext);
  const id = useId();
  const registerOutlet = context?.registerOutlet;

  useLayoutEffect(() => registerOutlet?.(id), [id, registerOutlet]);

  if (!context) return null;
  const active = context.activeOutletId === id;
  const content = active ? context.content : null;

  return (
    <>
      {active && <ToastAnnouncementRegion announcement={context.announcement} />}
      <div
        data-slot="modal-toast-outlet"
        data-testid="modal-toast-outlet"
        className={cn(
          "max-h-[min(11rem,22dvh)] min-h-0 w-full min-w-0 overscroll-contain empty:hidden",
          className,
        )}
        style={{ overflowY: content ? "auto" : "hidden" }}
      >
        {content}
      </div>
    </>
  );
}
