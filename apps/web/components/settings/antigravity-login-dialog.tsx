"use client";

import { useTranslation } from "react-i18next";
import { IconCopy, IconExternalLink, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { isAntigravityLoginActive, useAntigravityLogin } from "./use-antigravity-login";

function statusKey(state: string): string {
  switch (state) {
    case "starting":
      return "agents:antigravityLoginStarting";
    case "waiting_for_browser":
      return "agents:antigravityLoginWaiting";
    case "checking":
      return "agents:antigravityLoginChecking";
    case "succeeded":
      return "agents:antigravityLoginSucceeded";
    case "failed":
      return "agents:antigravityLoginFailed";
    case "cancelled":
      return "agents:antigravityLoginCancelled";
    case "expired":
      return "agents:antigravityLoginExpired";
    default:
      return "agents:antigravityLoginIdle";
  }
}

type Login = ReturnType<typeof useAntigravityLogin>;

function LoginActions({
  login,
  isMobile,
  showStart,
  onCancel,
}: {
  login: Login;
  isMobile: boolean;
  showStart: boolean;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const { snapshot, busy, copied, start, copy, openURL } = login;
  const active = isAntigravityLoginActive(snapshot);
  return (
    <>
      {showStart && (
        <Button
          className={isMobile ? "h-12 min-h-12 w-full" : undefined}
          onClick={start}
          disabled={busy}
        >
          {busy && <IconLoader2 className="mr-1 h-4 w-4 animate-spin" />}
          {snapshot ? t("agents:antigravityLoginAgain") : t("agents:antigravityLoginStart")}
        </Button>
      )}
      {snapshot?.authorization_url && !isMobile && (
        <>
          <Button onClick={openURL}>
            <IconExternalLink className="mr-1 h-4 w-4" />
            {t("agents:antigravityLoginOpen")}
          </Button>
          <Button variant="outline" onClick={copy}>
            <IconCopy className="mr-1 h-4 w-4" />
            {copied ? t("agents:antigravityLoginCopied") : t("agents:antigravityLoginCopy")}
          </Button>
        </>
      )}
      {snapshot?.authorization_url && isMobile && (
        <Button className="h-12 min-h-12 w-full" variant="outline" onClick={copy}>
          <IconCopy className="mr-1 h-4 w-4" />
          {copied ? t("agents:antigravityLoginCopied") : t("agents:antigravityLoginCopy")}
        </Button>
      )}
      <Button
        className={isMobile ? "h-12 min-h-12 w-full" : undefined}
        variant="outline"
        onClick={onCancel}
        disabled={busy}
      >
        {active ? t("agents:antigravityLoginCancel") : t("common:close")}
      </Button>
    </>
  );
}

function LoginBody({ login }: { login: Login }) {
  const { t } = useTranslation();
  const { snapshot, error } = login;
  const active = isAntigravityLoginActive(snapshot);
  const catalogFailed =
    snapshot?.state === "succeeded" && snapshot?.error_code === "catalog_unavailable";
  return (
    <div className="space-y-4 px-4 pb-2 text-sm">
      <p className="text-muted-foreground">{t("agents:antigravityLoginDescription")}</p>
      <p className="text-muted-foreground">{t("agents:antigravityLoginSeparateStore")}</p>
      <p className="text-muted-foreground">{t("agents:antigravityLoginSameMachine")}</p>
      {snapshot && (
        <p
          className="flex items-center gap-2 font-medium"
          role="status"
          aria-live="polite"
          data-testid="antigravity-login-status"
        >
          {active && <IconLoader2 className="h-4 w-4 animate-spin" />}
          {t(statusKey(snapshot.state))}
        </p>
      )}
      {catalogFailed && (
        <p className="text-amber-600 dark:text-amber-400" role="alert">
          {t("agents:antigravityLoginCatalogUnavailable")}
        </p>
      )}
      {(error || snapshot?.error) && (
        <p className="text-destructive" role="alert">
          {error ?? snapshot?.error}
        </p>
      )}
    </div>
  );
}

export function AntigravityLoginDialog({
  open,
  onOpenChange,
  onComplete,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onComplete?: () => void;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const login = useAntigravityLogin({ open, onComplete });
  const active = isAntigravityLoginActive(login.snapshot);
  const showStart = !active && login.snapshot?.state !== "succeeded";
  const handleCancel = async () => {
    if (await login.cancel()) onOpenChange(false);
  };
  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) {
      onOpenChange(true);
      return;
    }
    void login.dismiss().then((dismissed) => {
      if (dismissed) onOpenChange(false);
    });
  };
  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={handleOpenChange}>
        <DrawerContent className="max-h-[92dvh]" data-testid="antigravity-login-sheet">
          <DrawerHeader className="text-left">
            <DrawerTitle>{t("agents:antigravityLoginTitle")}</DrawerTitle>
            <DrawerDescription className="sr-only">
              {t("agents:antigravityLoginDescription")}
            </DrawerDescription>
          </DrawerHeader>
          <div className="overflow-y-auto">
            <LoginBody login={login} />
          </div>
          <DrawerFooter>
            <LoginActions login={login} isMobile showStart={showStart} onCancel={handleCancel} />
          </DrawerFooter>
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-lg" data-testid="antigravity-login-dialog">
        <DialogHeader>
          <DialogTitle>{t("agents:antigravityLoginTitle")}</DialogTitle>
          <DialogDescription>{t("agents:antigravityLoginDescription")}</DialogDescription>
        </DialogHeader>
        <LoginBody login={login} />
        <DialogFooter>
          <LoginActions
            login={login}
            isMobile={false}
            showStart={showStart}
            onCancel={handleCancel}
          />
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
