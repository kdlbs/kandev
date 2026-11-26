import { IconFlagFilled } from "@tabler/icons-react";
import type { ComponentProps, SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

export function stopTaskFlagEvent(event: SyntheticEvent) {
  event.stopPropagation();
}

export function TaskFlagTrigger({ onClick, ...props }: ComponentProps<"button">) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      className="inline-flex size-5 shrink-0 items-center justify-center rounded-sm text-destructive focus-visible:outline focus-visible:outline-ring max-md:size-11 [@media(pointer:coarse)]:size-11"
      aria-label={t("sidebar:flag")}
      onPointerDown={stopTaskFlagEvent}
      onMouseDown={stopTaskFlagEvent}
      onTouchStart={stopTaskFlagEvent}
      {...props}
      onClick={(event) => {
        event.stopPropagation();
        onClick?.(event);
      }}
    >
      <IconFlagFilled className="size-3.5" aria-hidden="true" />
    </button>
  );
}
