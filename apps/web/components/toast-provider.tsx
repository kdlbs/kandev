"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import { IconCheck, IconX, IconInfoCircle, IconLoader2 } from "@tabler/icons-react";
import Link from "@/components/routing/app-link";
import { Button } from "@kandev/ui/button";
import {
  ModalToastOutletContext,
  ToastAnnouncementRegion,
  type ModalToastOutletContextValue,
} from "@kandev/ui/modal-toast-outlet";
import { cn, generateUUID } from "@/lib/utils";
import { scheduleFrontendErrorReport } from "@/lib/api/domains/frontend-error-log-api";
import { useTranslation } from "react-i18next";

type ToastVariant = "default" | "success" | "error" | "loading";
type ToastPlacement = "top";

type ToastAction = { label: string } & ({ href: string } | { onClick: () => void });

type Toast = {
  id: string;
  title?: string;
  description?: string;
  variant?: ToastVariant;
  placement?: ToastPlacement;
  /** A link (`href`) or a button (`onClick`, which also dismisses the toast). */
  action?: ToastAction;
};

type ToastInput = Omit<Toast, "id"> & { duration?: number };

type ToastAnnouncementQueue = {
  messages: string[];
  phase: "idle" | "clearing" | "announcing";
  timer: ReturnType<typeof setTimeout> | null;
};

type ToastContextValue = {
  toast: (input: ToastInput) => string;
  updateToast: (id: string, input: Partial<ToastInput>) => void;
  dismissToast: (id: string) => void;
};

const ToastContext = createContext<ToastContextValue | null>(null);

const ANNOUNCEMENT_RESTORE_DELAY_MS = 50;
const ANNOUNCEMENT_HOLD_MS = 750;

function scheduleNextToastAnnouncement(
  queue: ToastAnnouncementQueue,
  setAnnouncement: (message: string) => void,
) {
  if (queue.phase !== "idle" || queue.messages.length === 0) return;

  queue.phase = "clearing";
  setAnnouncement("");
  queue.timer = setTimeout(() => {
    queue.timer = null;
    const message = queue.messages.shift();
    if (message === undefined) {
      queue.phase = "idle";
      return;
    }

    queue.phase = "announcing";
    setAnnouncement(message);
    queue.timer = setTimeout(() => {
      queue.timer = null;
      queue.phase = "idle";
      scheduleNextToastAnnouncement(queue, setAnnouncement);
    }, ANNOUNCEMENT_HOLD_MS);
  }, ANNOUNCEMENT_RESTORE_DELAY_MS);
}

function useToastAnnouncementQueue() {
  const [announcement, setAnnouncement] = useState("");
  const queueRef = useRef<ToastAnnouncementQueue>({
    messages: [],
    phase: "idle",
    timer: null,
  });

  useEffect(
    () => () => {
      const timer = queueRef.current.timer;
      if (timer !== null) clearTimeout(timer);
    },
    [],
  );

  const announceToast = useCallback((toast: Toast) => {
    const queue = queueRef.current;
    queue.messages.push(
      [toast.title, toast.description, toast.action?.label].filter(Boolean).join(". "),
    );
    if (queue.phase === "clearing" && queue.timer !== null) {
      clearTimeout(queue.timer);
      queue.timer = null;
      queue.phase = "idle";
    }
    scheduleNextToastAnnouncement(queue, setAnnouncement);
  }, []);

  return { announcement, announceToast };
}

const variantStyles: Record<
  ToastVariant,
  { container: string; icon: string; IconComponent: typeof IconCheck; spin?: boolean }
> = {
  default: {
    container: "border-border/60 bg-background",
    icon: "text-muted-foreground",
    IconComponent: IconInfoCircle,
  },
  loading: {
    container: "border-border/60 bg-background",
    icon: "text-muted-foreground",
    IconComponent: IconLoader2,
    spin: true,
  },
  success: {
    container: "border-green-500/30 bg-green-500/10 dark:bg-green-500/5",
    icon: "text-green-600 dark:text-green-400",
    IconComponent: IconCheck,
  },
  error: {
    container: "border-red-500/30 bg-red-500/10 dark:bg-red-500/5",
    icon: "text-red-600 dark:text-red-400",
    IconComponent: IconX,
  },
};

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const { announcement, announceToast } = useToastAnnouncementQueue();
  const timersRef = useRef(new Map<string, ReturnType<typeof setTimeout>>());
  const toastsRef = useRef(new Map<string, Toast>());

  const removeToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((toast) => toast.id !== id));
    timersRef.current.delete(id);
    toastsRef.current.delete(id);
  }, []);

  const scheduleRemoval = useCallback(
    (id: string, duration: number) => {
      const existing = timersRef.current.get(id);
      if (existing) clearTimeout(existing);
      const timer = setTimeout(() => removeToast(id), duration);
      timersRef.current.set(id, timer);
    },
    [removeToast],
  );

  const toast = useCallback(
    (input: ToastInput): string => {
      const id = generateUUID();
      const nextToast: Toast = {
        id,
        title: input.title,
        description: input.description,
        variant: input.variant ?? "default",
        placement: input.placement,
        action: input.action,
      };
      toastsRef.current.set(id, nextToast);
      setToasts((prev) => [...prev, nextToast]);
      announceToast(nextToast);
      if (nextToast.variant === "error") {
        reportToastError(nextToast);
      }
      // Loading toasts don't auto-dismiss
      if (input.variant !== "loading") {
        scheduleRemoval(id, input.duration ?? 4000);
      }
      return id;
    },
    [announceToast, scheduleRemoval],
  );

  const updateToast = useCallback(
    (id: string, input: Partial<ToastInput>) => {
      const previous = toastsRef.current.get(id);
      if (previous) {
        const next = {
          ...previous,
          ...(input.title !== undefined && { title: input.title }),
          ...(input.description !== undefined && { description: input.description }),
          ...(input.variant !== undefined && { variant: input.variant }),
          ...(input.placement !== undefined && { placement: input.placement }),
          ...(input.action !== undefined && { action: input.action }),
        };
        toastsRef.current.set(id, next);
        setToasts((current) => current.map((item) => (item.id === id ? next : item)));
        announceToast(next);
        if (next.variant === "error" && previous.variant !== "error") {
          reportToastError(next);
        }
      }
      // When transitioning away from loading, schedule auto-dismiss
      if (input.variant && input.variant !== "loading") {
        scheduleRemoval(id, input.duration ?? 4000);
      }
    },
    [announceToast, scheduleRemoval],
  );

  const dismissToast = removeToast;
  const value = useMemo(
    () => ({ toast, updateToast, dismissToast }),
    [toast, updateToast, dismissToast],
  );

  return (
    <ToastContext.Provider value={value}>
      <ModalToastProvider toasts={toasts} announcement={announcement} dismissToast={dismissToast}>
        {children}
      </ModalToastProvider>
    </ToastContext.Provider>
  );
}

function ModalToastProvider({
  children,
  toasts,
  announcement,
  dismissToast,
}: {
  children: React.ReactNode;
  toasts: Toast[];
  announcement: string;
  dismissToast: (id: string) => void;
}) {
  const modalOutletsRef = useRef(new Set<string>());
  const [activeModalOutletId, setActiveModalOutletId] = useState<string | null>(null);
  const registerModalOutlet = useCallback((id: string) => {
    const outlets = modalOutletsRef.current;
    outlets.delete(id);
    outlets.add(id);
    setActiveModalOutletId(id);
    return () => {
      outlets.delete(id);
      setActiveModalOutletId(Array.from(outlets).at(-1) ?? null);
    };
  }, []);
  const modalContent = useMemo(
    () =>
      toasts.length > 0 ? <ModalToastContent toasts={toasts} dismissToast={dismissToast} /> : null,
    [dismissToast, toasts],
  );
  const modalOutletContext = useMemo<ModalToastOutletContextValue>(
    () => ({
      activeOutletId: activeModalOutletId,
      content: modalContent,
      announcement,
      registerOutlet: registerModalOutlet,
    }),
    [activeModalOutletId, announcement, modalContent, registerModalOutlet],
  );

  return (
    <ModalToastOutletContext.Provider value={modalOutletContext}>
      {children}
      {activeModalOutletId === null && <ToastAnnouncementRegion announcement={announcement} />}
      <ToastList
        visible={activeModalOutletId === null}
        toasts={toasts}
        dismissToast={dismissToast}
      />
    </ModalToastOutletContext.Provider>
  );
}

function reportToastError(toast: Toast) {
  scheduleFrontendErrorReport({
    source: "toast-provider",
    title: toast.title,
    description: toast.description,
  });
}

function ToastActionButton({
  action,
  onDone,
}: {
  action: { label: string; onClick: () => void };
  onDone: () => void;
}) {
  return (
    <button
      type="button"
      className="pointer-events-auto cursor-pointer text-xs font-medium underline max-md:min-h-11 pointer-coarse:min-h-11"
      onClick={() => {
        action.onClick();
        onDone();
      }}
    >
      {action.label}
    </button>
  );
}

function ToastList({
  visible,
  toasts,
  dismissToast,
}: {
  visible: boolean;
  toasts: Toast[];
  dismissToast: (id: string) => void;
}) {
  if (!visible) return null;
  const bottomToasts = toasts.filter((toast) => toast.placement !== "top");
  const topToasts = toasts.filter((toast) => toast.placement === "top");
  return (
    <div className="pointer-events-none fixed inset-0 z-[60]" data-testid="toast-container">
      <ToastStack
        dismissToast={dismissToast}
        toasts={bottomToasts}
        className="absolute bottom-[calc(1rem+var(--app-status-bar-height))] right-4 flex w-[calc(100vw-2rem)] max-w-[360px] flex-col-reverse gap-2"
      />
      <ToastStack
        dismissToast={dismissToast}
        toasts={topToasts}
        className="absolute top-[calc(3.25rem+env(safe-area-inset-top,0px)+var(--app-status-bar-height))] right-4 flex w-[calc(100vw-2rem)] max-w-[360px] flex-col-reverse gap-2"
      />
    </div>
  );
}

function ModalToastContent({
  toasts,
  dismissToast,
}: {
  toasts: Toast[];
  dismissToast: (id: string) => void;
}) {
  return (
    <div className="pointer-events-none" data-testid="toast-container">
      <ToastStack
        dismissToast={dismissToast}
        toasts={toasts}
        className="ml-auto flex w-full max-w-[360px] flex-col-reverse gap-2 max-md:max-w-none"
      />
    </div>
  );
}

function ToastStack({
  toasts,
  className,
  dismissToast,
}: {
  toasts: Toast[];
  className: string;
  dismissToast: (id: string) => void;
}) {
  const { t } = useTranslation();
  if (toasts.length === 0) return null;
  return (
    <div className={cn("pointer-events-none", className)} onKeyDown={stopToastEnterPropagation}>
      {toasts.map((toast) => {
        const variant = toast.variant ?? "default";
        const styles = variantStyles[variant];
        const Icon = styles.IconComponent;
        return (
          <div
            key={toast.id}
            data-testid="toast-message"
            className={cn(
              "pointer-events-none flex items-start gap-3 rounded-lg border px-4 py-3 shadow-lg backdrop-blur-sm",
              "animate-in slide-in-from-right-full duration-300",
              styles.container,
            )}
          >
            <div className={cn("mt-0.5 flex-shrink-0", styles.icon)}>
              <Icon aria-hidden="true" className={cn("h-5 w-5", styles.spin && "animate-spin")} />
            </div>
            <div className="min-w-0 flex-1 space-y-1">
              {toast.title && (
                <div className="text-sm font-semibold leading-tight">{toast.title}</div>
              )}
              {toast.action && "href" in toast.action && (
                <Button
                  asChild
                  size="sm"
                  variant="outline"
                  className="pointer-events-auto h-7 min-h-7 max-md:h-11 max-md:min-h-11 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:min-h-11"
                >
                  <Link href={toast.action.href}>{toast.action.label}</Link>
                </Button>
              )}
              {toast.description && (
                <div className="text-xs leading-relaxed text-muted-foreground">
                  {toast.description}
                </div>
              )}
              {toast.action && "onClick" in toast.action && (
                <ToastActionButton action={toast.action} onDone={() => dismissToast(toast.id)} />
              )}
            </div>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={t("common:dismissNotification")}
              className="pointer-events-auto"
              onClick={() => dismissToast(toast.id)}
            >
              <IconX aria-hidden="true" />
            </Button>
          </div>
        );
      })}
    </div>
  );
}

function stopToastEnterPropagation(event: ReactKeyboardEvent<HTMLDivElement>) {
  if (
    event.key === "Enter" &&
    event.target instanceof Element &&
    event.target.closest("button, a")
  ) {
    event.stopPropagation();
  }
}

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error("useToast must be used within ToastProvider");
  }
  return context;
}

/** Returns null outside a provider so optional surfaces can degrade silently. */
export function useOptionalToast() {
  return useContext(ToastContext);
}
