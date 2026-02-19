"use client";

import { createContext, useCallback, useContext, useMemo, useRef, useState } from "react";
import { IconCheck, IconX, IconInfoCircle, IconLoader2 } from "@tabler/icons-react";
import { cn, generateUUID } from "@/lib/utils";

type ToastVariant = "default" | "success" | "error" | "loading";

type Toast = {
  id: string;
  title?: string;
  description?: string;
  variant?: ToastVariant;
};

type ToastInput = Omit<Toast, "id"> & { duration?: number };

type ToastContextValue = {
  toast: (input: ToastInput) => string;
  updateToast: (id: string, input: Partial<ToastInput>) => void;
  dismissToast: (id: string) => void;
};

const ToastContext = createContext<ToastContextValue | null>(null);

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
  const timersRef = useRef(new Map<string, ReturnType<typeof setTimeout>>());

  const removeToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((toast) => toast.id !== id));
    timersRef.current.delete(id);
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
      };
      setToasts((prev) => [...prev, nextToast]);
      // Loading toasts don't auto-dismiss
      if (input.variant !== "loading") {
        scheduleRemoval(id, input.duration ?? 4000);
      }
      return id;
    },
    [scheduleRemoval],
  );

  const updateToast = useCallback(
    (id: string, input: Partial<ToastInput>) => {
      setToasts((prev) =>
        prev.map((t) =>
          t.id === id
            ? {
                ...t,
                ...(input.title !== undefined && { title: input.title }),
                ...(input.description !== undefined && { description: input.description }),
                ...(input.variant !== undefined && { variant: input.variant }),
              }
            : t,
        ),
      );
      // When transitioning away from loading, schedule auto-dismiss
      if (input.variant && input.variant !== "loading") {
        scheduleRemoval(id, input.duration ?? 4000);
      }
    },
    [scheduleRemoval],
  );

  const dismissToast = useCallback(
    (id: string) => {
      removeToast(id);
    },
    [removeToast],
  );

  const value = useMemo(
    () => ({ toast, updateToast, dismissToast }),
    [toast, updateToast, dismissToast],
  );

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="fixed bottom-4 right-4 z-50 flex w-[360px] flex-col-reverse gap-2">
        {toasts.map((t) => {
          const variant = t.variant ?? "default";
          const styles = variantStyles[variant];
          const Icon = styles.IconComponent;
          return (
            <div
              key={t.id}
              className={cn(
                "flex items-start gap-3 rounded-lg border px-4 py-3 shadow-lg backdrop-blur-sm",
                "animate-in slide-in-from-right-full duration-300",
                styles.container,
              )}
            >
              <div className={cn("mt-0.5 flex-shrink-0", styles.icon)}>
                <Icon className={cn("h-5 w-5", styles.spin && "animate-spin")} />
              </div>
              <div className="flex-1 space-y-1">
                {t.title && <div className="text-sm font-semibold leading-tight">{t.title}</div>}
                {t.description && (
                  <div className="text-xs leading-relaxed text-muted-foreground">
                    {t.description}
                  </div>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error("useToast must be used within ToastProvider");
  }
  return context;
}
