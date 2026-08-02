import { IconChevronLeft, IconChevronRight } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";

type Props = {
  skip: number;
  count: number;
  loading: boolean;
  pageSize: number;
  onPage: (skip: number) => void;
};

export function AzureDevOpsPullRequestPagination({
  skip,
  count,
  loading,
  pageSize,
  onPage,
}: Props) {
  if (skip === 0 && count < pageSize) return null;
  return (
    <div className="flex items-center justify-between border-t px-4 py-2">
      <span className="text-xs text-muted-foreground">
        {count === 0 ? 0 : skip + 1}-{skip + count}
      </span>
      <div className="flex gap-1">
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          onClick={() => onPage(Math.max(0, skip - pageSize))}
          disabled={loading || skip === 0}
          className="cursor-pointer"
          aria-label="Previous pull request page"
        >
          <IconChevronLeft className="h-4 w-4" />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          onClick={() => onPage(skip + pageSize)}
          disabled={loading || count < pageSize}
          className="cursor-pointer"
          aria-label="Next pull request page"
        >
          <IconChevronRight className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}
