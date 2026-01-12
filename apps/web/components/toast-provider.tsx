'use client';

import { createContext, useCallback, useContext, useMemo, useState } from 'react';
import { cn, generateUUID } from '@/lib/utils';

type ToastVariant = 'default' | 'error';

type Toast = {
  id: string;
  title?: string;
  description?: string;
  variant?: ToastVariant;
};

type ToastInput = Omit<Toast, 'id'> & { duration?: number };

type ToastContextValue = {
  toast: (input: ToastInput) => void;
};

const ToastContext = createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const removeToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((toast) => toast.id !== id));
  }, []);

  const toast = useCallback(
    (input: ToastInput) => {
      const id = generateUUID();
      const nextToast: Toast = {
        id,
        title: input.title,
        description: input.description,
        variant: input.variant ?? 'default',
      };
      setToasts((prev) => [nextToast, ...prev]);
      const duration = input.duration ?? 3000;
      window.setTimeout(() => removeToast(id), duration);
    },
    [removeToast]
  );

  const value = useMemo(() => ({ toast }), [toast]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="fixed right-4 top-4 z-50 flex w-[320px] flex-col gap-2">
        {toasts.map((toast) => (
          <div
            key={toast.id}
            className={cn(
              'rounded-md border border-border/60 bg-background px-3 py-2 shadow-md',
              toast.variant === 'error' && 'border-destructive/50 bg-destructive/10'
            )}
          >
            {toast.title && <div className="text-sm font-medium">{toast.title}</div>}
            {toast.description && (
              <div className="text-xs text-muted-foreground">{toast.description}</div>
            )}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error('useToast must be used within ToastProvider');
  }
  return context;
}
