import type { ComponentPropsWithoutRef, ReactNode } from "react";
import type { TablerIcon } from "@tabler/icons-react";

export function ComposerWarning(props: Omit<ComponentPropsWithoutRef<"div">, "className">) {
  return (
    <div
      {...props}
      className="border-border/60 bg-amber-500/5 my-1 flex flex-col gap-1 rounded-md border px-2 py-2"
    />
  );
}

export function ComposerWarningRow({
  icon: Icon,
  children,
}: {
  icon: TablerIcon;
  children: ReactNode;
}) {
  return (
    <div className="flex min-w-0 items-center gap-2">
      <Icon className="size-4 shrink-0 text-amber-600" aria-hidden="true" />
      {children}
    </div>
  );
}
