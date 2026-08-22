import { IconDots } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";

export function TaskMenuButton({
  visible,
  expanded,
  rowFocus = false,
}: {
  visible: boolean;
  expanded: boolean;
  rowFocus?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div
      className={cn(
        "mobile-task-actions self-center shrink-0 flex items-center transition-opacity duration-100",
        !visible && "[@media(hover:none)]:hidden",
        visible
          ? "opacity-100"
          : cn(
              "opacity-0 pointer-events-none [@media(hover:hover)]:group-hover:opacity-100 [@media(hover:hover)]:group-hover:pointer-events-auto",
              rowFocus
                ? "group-focus-within:opacity-100 group-focus-within:pointer-events-auto"
                : "focus-within:opacity-100 focus-within:pointer-events-auto",
            ),
      )}
    >
      <button
        type="button"
        className={cn(
          "mobile-task-actions-button flex size-6 items-center justify-center rounded-md cursor-pointer touch-manipulation",
          "text-muted-foreground hover:text-foreground hover:bg-foreground/10",
          "focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring transition-colors",
        )}
        onClick={(e) => {
          e.stopPropagation();
          e.preventDefault();
          e.currentTarget.dispatchEvent(
            new MouseEvent("contextmenu", {
              bubbles: true,
              clientX: e.clientX,
              clientY: e.clientY,
            }),
          );
        }}
        aria-label={t("task:taskActions")}
        aria-haspopup="menu"
        aria-expanded={expanded}
      >
        <IconDots className="h-4 w-4" />
      </button>
    </div>
  );
}
