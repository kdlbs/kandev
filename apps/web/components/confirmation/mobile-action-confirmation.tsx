"use client";

import { useId, useLayoutEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import { createPortal } from "react-dom";
import { Drawer, DrawerContent, DrawerTitle, DrawerDescription } from "@kandev/ui/drawer";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  MobileConfirmationContent,
  type MobileConfirmationContentProps,
} from "./mobile-confirmation-content";
import { useMobileConfirmationHost } from "./mobile-confirmation-host";
import {
  useMobileConfirmationSubmit,
  type ConfirmationCompletionPolicy,
} from "./use-mobile-confirmation-submit";

export type MobileActionConfirmationProps = Omit<
  MobileConfirmationContentProps,
  "onConfirm" | "onCancel"
> & {
  open: boolean;
  targetKey: string;
  onOpenChange: (open: boolean) => void;
  onCancel?: () => void;
  /** Local owner dismissal before dispatch, when it differs from cancellation. */
  onClose?: () => void;
  onConfirm: () => void | Promise<void>;
  completionPolicy?: ConfirmationCompletionPolicy;
  focusReturnRef?: RefObject<HTMLElement | null>;
  fallback?: ReactNode;
};

/** Keep this adapter mounted across responsive branches so a boundary change cancels the request. */
export function MobileActionConfirmation(props: MobileActionConfirmationProps) {
  const { isMobile, changed } = useConfirmationBoundary(
    props.open,
    props.targetKey,
    props.onOpenChange,
  );
  if (changed || !props.open) return null;
  if (!isMobile) return props.fallback ?? null;
  return <OpenMobileConfirmation {...props} />;
}

export function useConfirmationBoundary(
  open: boolean,
  targetKey: string,
  onOpenChange: (open: boolean) => void,
) {
  const viewport = useResponsiveBreakpoint();
  const { isMobile } = viewport;
  const previous = useRef({ isMobile, targetKey, open });
  const changed =
    previous.current.open &&
    open &&
    (previous.current.isMobile !== isMobile || previous.current.targetKey !== targetKey);
  useLayoutEffect(() => {
    previous.current = { isMobile, targetKey, open };
    if (changed && open) onOpenChange(false);
  });
  return { ...viewport, changed };
}

function OpenMobileConfirmation({
  onOpenChange,
  onCancel,
  onClose,
  onConfirm,
  completionPolicy = "close-before-dispatch",
  focusReturnRef,
  fallback: _fallback,
  open: _open,
  targetKey: _targetKey,
  ...content
}: MobileActionConfirmationProps) {
  const host = useMobileConfirmationHost();
  const id = useId();
  const token = useRef(Symbol());
  const [closed, setClosed] = useState(false);
  const { submit, submitted, pending } = useMobileConfirmationSubmit({
    closed,
    disabled: !!(content.disabled || content.confirmDisabled),
    onConfirm,
    completionPolicy,
    close: () => {
      setClosed(true);
      if (onClose) onClose();
      else onOpenChange(false);
    },
  });
  const initialFocus = useRef(document.activeElement as HTMLElement | null);
  const latest = useRef({ onOpenChange, onCancel, disabled: content.disabled });
  useLayoutEffect(() => {
    latest.current = { onOpenChange, onCancel, disabled: content.disabled };
  });
  const titleId = `${id}-title`;
  const descriptionId = `${id}-description`;

  const cancel = useRef((invalidated = false) => {
    if (!invalidated && (submitted.current || latest.current.disabled)) return;
    setClosed(true);
    latest.current.onCancel?.();
    latest.current.onOpenChange(false);
    queueMicrotask(() => {
      const target = focusReturnRef?.current ?? initialFocus.current;
      if (host) host.restoreFocus(target);
      else if (target?.isConnected && !target.closest("[inert]"))
        target.focus({ preventScroll: true });
    });
  }).current;
  const register = host?.register;
  const release = host?.release;
  useLayoutEffect(() => {
    if (!register || !release || closed) return;
    const requestToken = token.current;
    register({ token: requestToken, titleId, descriptionId, cancel });
    return () => release(requestToken);
  }, [register, release, closed, titleId, descriptionId, cancel]);

  const body = closed ? null : (
    <MobileConfirmationContent
      {...content}
      disabled={content.disabled || pending}
      titleId={titleId}
      descriptionId={descriptionId}
      onBack={host ? () => cancel() : undefined}
      onCancel={() => cancel()}
      onConfirm={submit}
    />
  );
  if (host)
    return host.outlet && host.request?.token === token.current
      ? createPortal(body, host.outlet)
      : null;
  return (
    <Drawer
      open={!closed}
      onOpenChange={(next) => {
        if (!next) cancel();
      }}
    >
      <DrawerContent
        aria-labelledby={titleId}
        aria-describedby={descriptionId}
        className="data-[vaul-drawer-direction=bottom]:max-h-[calc(100dvh-1rem)] data-[vaul-drawer-direction=bottom]:mt-0 overflow-hidden"
        onOpenAutoFocus={(event) => event.preventDefault()}
        onCloseAutoFocus={(event) => event.preventDefault()}
        onEscapeKeyDown={(event) => {
          event.preventDefault();
          cancel();
        }}
      >
        <DrawerTitle asChild>
          <span hidden>{content.title}</span>
        </DrawerTitle>
        <DrawerDescription asChild>
          <span hidden>{content.subject ?? content.title}</span>
        </DrawerDescription>
        {body}
      </DrawerContent>
    </Drawer>
  );
}
