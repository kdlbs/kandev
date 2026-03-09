import { IconMessageCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";

/** Quick Chat button that opens the quick chat modal */
export function QuickChatButton({ workspaceId }: { workspaceId?: string | null }) {
  const handleOpenQuickChat = useQuickChatLauncher(workspaceId);

  if (!workspaceId) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="outline"
          className="cursor-pointer px-2"
          onClick={handleOpenQuickChat}
        >
          <IconMessageCircle className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>Quick Chat</TooltipContent>
    </Tooltip>
  );
}
