import type { ReactNode } from "react";
import { AddToContextButton, ExpandableBody, formatTimeAgo } from "@/components/github/pr-shared";
import { ChangeRequestPersonLink } from "./change-request-detail-header";
import type { ChangeRequestDetailPerson } from "./change-request-detail";

function PersonAvatar({ person }: { person: ChangeRequestDetailPerson }) {
  if (person.avatarUrl) {
    return <img src={person.avatarUrl} alt="" className="h-5 w-5 shrink-0 rounded-full" />;
  }
  return (
    <div className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-muted text-[10px] font-medium text-muted-foreground">
      {person.name[0]?.toUpperCase() ?? "?"}
    </div>
  );
}

export function ChangeRequestFeedbackRow({
  person,
  body,
  createdAt,
  metadata,
  onAdd,
  reply,
}: {
  person: ChangeRequestDetailPerson;
  body?: string;
  createdAt?: string;
  metadata?: ReactNode;
  onAdd?: () => void;
  reply?: boolean;
}) {
  return (
    <div className={reply ? "ml-4 border-l-2 border-border pl-2.5" : ""}>
      <div className="space-y-1 rounded-md border border-border bg-muted/30 px-2.5 py-2">
        <div className="block sm:flex sm:flex-nowrap sm:items-center sm:gap-2">
          <PersonAvatar person={person} />
          <ChangeRequestPersonLink person={person} />
          {metadata}
          {createdAt ? (
            <span className="ml-auto shrink-0 text-[10px] text-muted-foreground">
              {formatTimeAgo(createdAt)}
            </span>
          ) : null}
          {onAdd ? (
            <div className="[&_button]:h-11 [&_button]:w-11 sm:[&_button]:h-6 sm:[&_button]:w-6">
              <AddToContextButton onClick={onAdd} />
            </div>
          ) : null}
        </div>
        {body ? <ExpandableBody body={body} className="pl-7" /> : null}
      </div>
    </div>
  );
}

export function normalizeChangeRequestPersonName(name: string) {
  return name
    .trim()
    .toLowerCase()
    .replaceAll(/[^a-z0-9_-]/g, "-");
}
