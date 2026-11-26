"use client";

export function ChangesPanelFileName({
  name,
  touchMode = false,
}: {
  name: string;
  touchMode?: boolean;
}) {
  return (
    <span
      data-testid="changes-file-name"
      className={
        touchMode
          ? "block whitespace-normal [overflow-wrap:anywhere] text-xs font-medium leading-4"
          : "font-medium text-foreground truncate min-w-0"
      }
    >
      {name}
    </span>
  );
}

export function ChangesPanelFileFolder({
  folder,
  touchMode = false,
  trailingSlash = false,
}: {
  folder: string;
  touchMode?: boolean;
  trailingSlash?: boolean;
}) {
  return (
    <span
      className={
        touchMode ? "min-w-0 truncate" : "text-foreground/60 truncate min-w-0 [flex-shrink:9999]"
      }
    >
      {folder}
      {trailingSlash ? "/" : ""}
    </span>
  );
}
