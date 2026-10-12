import { forwardRef } from "react";
import { useTranslation } from "react-i18next";
import { IconMessageChatbot } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

type WorkspaceCopilotLauncherProps = {
  onOpen: () => void;
  /** A task page shows the walkthrough launcher in the same corner. */
  aboveWalkthrough: boolean;
};

/**
 * The workspace host's closed launcher. It names no coordinator and shows no
 * working state; the coordinator is chosen when the panel opens. Fixed at the
 * bottom right, above the walkthrough launcher where that shows, and on a phone
 * above the session bottom navigation and the safe-area inset.
 */
export const WorkspaceCopilotLauncher = forwardRef<
  HTMLButtonElement,
  WorkspaceCopilotLauncherProps
>(function WorkspaceCopilotLauncher({ onOpen, aboveWalkthrough }, ref) {
  const { t } = useTranslation();
  const label = t("coordinator:copilotWorkspaceLauncher");
  const offset = aboveWalkthrough
    ? "bottom-[calc(5.5rem+var(--app-status-bar-height))]"
    : "bottom-[calc(1.5rem+var(--app-status-bar-height))]";
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          ref={ref}
          size="icon"
          className={`fixed ${offset} right-6 z-[60] size-12 cursor-pointer rounded-full shadow-lg max-md:bottom-[calc(4.5rem+env(safe-area-inset-bottom)+var(--app-status-bar-height))] [@media(pointer:coarse)]:size-12`}
          aria-label={label}
          data-testid="workspace-copilot-launcher"
          onClick={onOpen}
        >
          <IconMessageChatbot className="h-6 w-6" />
        </Button>
      </TooltipTrigger>
      <TooltipContent side="left">{label}</TooltipContent>
    </Tooltip>
  );
});
