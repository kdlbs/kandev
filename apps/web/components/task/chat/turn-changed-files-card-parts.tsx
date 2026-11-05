import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

export function CardNotice({ text, role }: { text: string; role?: "status" | "alert" }) {
  return (
    <p
      className={
        role === "alert"
          ? "px-3 py-2 text-xs text-destructive"
          : "px-3 py-2 text-xs text-muted-foreground"
      }
      role={role}
    >
      {text}
    </p>
  );
}

export function LoadMoreButton({ loading, onClick }: { loading: boolean; onClick: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="px-3 py-2">
      <Button
        type="button"
        size="sm"
        variant="ghost"
        className="h-7 min-h-7 cursor-pointer px-2 text-xs max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        disabled={loading}
        onClick={onClick}
      >
        {loading ? t("task:turnChangesLoading") : t("task:turnChangesLoadMore")}
      </Button>
    </div>
  );
}

export function RetryFilesButton({ onClick }: { onClick: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="px-3 pb-2">
      <Button
        size="sm"
        variant="ghost"
        className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        onClick={onClick}
      >
        {t("task:turnChangesRetry")}
      </Button>
    </div>
  );
}
