import { cn } from "@kandev/ui/lib/utils";

export type Graph2ConnectorProps = {
  type: "past" | "transition" | "future";
};

export function Graph2Connector({ type }: Graph2ConnectorProps) {
  return (
    <div className="flex items-center shrink-0">
      <div
        className={cn(
          "w-5 h-px",
          type === "past" && "bg-muted-foreground/40",
          type === "transition" &&
            "bg-muted-foreground/30 border-t border-dashed border-muted-foreground/30",
          type === "future" && "border-t border-dashed border-muted-foreground/20",
        )}
      />
      <div
        className={cn(
          "w-0 h-0 border-t-[3px] border-t-transparent border-b-[3px] border-b-transparent border-l-[5px]",
          type === "past" && "border-l-muted-foreground/40",
          type === "transition" && "border-l-muted-foreground/30",
          type === "future" && "border-l-muted-foreground/20",
        )}
      />
    </div>
  );
}
