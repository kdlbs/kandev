import { IconFlagFilled } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

export function TaskFlagIndicator({
  note,
  onOpen,
}: {
  note: string;
  onOpen?: (invoker: HTMLButtonElement) => void;
}) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const touch = isMobile || !isFinePointer;
  const stop = (event: React.SyntheticEvent) => event.stopPropagation();
  const button = (
    <button
      type="button"
      className="inline-flex size-5 shrink-0 items-center justify-center rounded-sm text-destructive focus-visible:outline focus-visible:outline-ring max-md:size-11 [@media(pointer:coarse)]:size-11"
      aria-label={t("sidebar:flag")}
      aria-haspopup={touch && onOpen ? "dialog" : undefined}
      aria-description={touch ? note : undefined}
      onPointerDown={stop}
      onMouseDown={stop}
      onTouchStart={stop}
      onClick={(event) => {
        event.stopPropagation();
        if (touch) onOpen?.(event.currentTarget);
      }}
    >
      <IconFlagFilled className="size-3.5" aria-hidden="true" />
    </button>
  );
  if (touch && onOpen) return button;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{button}</TooltipTrigger>
      <TooltipContent className="whitespace-pre-wrap wrap-anywhere">{note}</TooltipContent>
    </Tooltip>
  );
}
