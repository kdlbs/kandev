import { Button } from "@kandev/ui/button";
import { IconX } from "@tabler/icons-react";
import { CommandDialog } from "@kandev/ui/command";
import type { ComponentProps } from "react";
import { Drawer, DrawerContent, DrawerTitle } from "@kandev/ui/drawer";
import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { cn } from "@/lib/utils";
import {
  MobileConfirmationHost,
  MobileConfirmationHostBody,
} from "./confirmation/mobile-confirmation-host";

export function CommandPanelDialog(props: ComponentProps<typeof CommandDialog>) {
  const { isMobile } = useResponsiveBreakpoint();
  const { t } = useTranslation();
  return (
    <MobileConfirmationHost open={Boolean(props.open)} surface={isMobile ? "drawer" : "dialog"}>
      {({ active, contentProps }) =>
        isMobile ? (
          <Drawer
            open={props.open}
            onOpenChange={props.onOpenChange}
            shouldScaleBackground={false}
            autoFocus
            repositionInputs={false}
          >
            {props.open && (
              <DrawerContent
                {...contentProps}
                // Closing a selection must not dismiss the dialog beneath the palette.
                onClick={(event) => event.stopPropagation()}
                onEscapeKeyDown={
                  active ? contentProps.onEscapeKeyDown : props.contentProps?.onEscapeKeyDown
                }
                className="mx-2 mb-[max(0.5rem,env(safe-area-inset-bottom))] h-[85dvh] data-[vaul-drawer-direction=bottom]:max-h-[85dvh] overflow-hidden rounded-xl bg-background p-0 [&_[data-slot=command-list]]:max-h-none [&_[data-slot=command-list]]:min-h-0 [&_[data-slot=command-list]]:flex-1"
                aria-describedby={contentProps["aria-describedby"]}
              >
                <DrawerTitle className="sr-only" aria-hidden={active || undefined}>
                  {t("common:conversationSearch_title")}
                </DrawerTitle>
                <div className="flex min-h-11 shrink-0 items-center justify-end px-2">
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-11"
                    aria-label={t("common:close")}
                    onClick={() => props.onOpenChange?.(false)}
                  >
                    <IconX className="size-4" aria-hidden />
                  </Button>
                </div>
                <MobileConfirmationHostBody>{props.children}</MobileConfirmationHostBody>
              </DrawerContent>
            )}
          </Drawer>
        ) : (
          <CommandDialog
            {...props}
            title={t("common:conversationSearch_title")}
            className={cn(props.className, active && "flex max-h-[calc(100dvh-2rem)] flex-col")}
            contentProps={{ ...contentProps, ...props.contentProps, enterConfirms: !active }}
          >
            <MobileConfirmationHostBody>{props.children}</MobileConfirmationHostBody>
          </CommandDialog>
        )
      }
    </MobileConfirmationHost>
  );
}
