import { CommandDialog } from "@kandev/ui/command";
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";
import {
  MobileConfirmationHost,
  MobileConfirmationHostBody,
} from "./confirmation/mobile-confirmation-host";

export function CommandPanelDialog(props: ComponentProps<typeof CommandDialog>) {
  return (
    <MobileConfirmationHost open={Boolean(props.open)}>
      {({ active, contentProps }) => (
        <CommandDialog
          {...props}
          className={cn(props.className, active && "flex max-h-[calc(100dvh-2rem)] flex-col")}
          contentProps={{ ...contentProps, enterConfirms: !active }}
        >
          <MobileConfirmationHostBody>{props.children}</MobileConfirmationHostBody>
        </CommandDialog>
      )}
    </MobileConfirmationHost>
  );
}
