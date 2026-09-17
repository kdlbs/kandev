import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";

export function CoordinatorTabs({ tab, setTab }: { tab: string; setTab: (tab: string) => void }) {
  const { t } = useTranslation();
  return (
    <div
      role="tablist"
      aria-label={t("orchestration:coordinator")}
      className="grid grid-cols-2 border-b"
      onKeyDown={(event) => {
        if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
        event.preventDefault();
        let next = tab === "tasks" ? "chat" : "tasks";
        if (event.key === "Home") next = "tasks";
        if (event.key === "End") next = "chat";
        setTab(next);
        event.currentTarget.querySelector<HTMLElement>(`#coordinator-tab-${next}`)?.focus();
      }}
    >
      {["tasks", "chat"].map((id) => (
        <Button
          key={id}
          id={`coordinator-tab-${id}`}
          role="tab"
          tabIndex={tab === id ? 0 : -1}
          aria-selected={tab === id}
          aria-controls={`coordinator-panel-${id}`}
          variant={tab === id ? "secondary" : "ghost"}
          className="cursor-pointer rounded-none min-h-11"
          onClick={() => setTab(id)}
        >
          {t(`orchestration:${id}Tab`)}
        </Button>
      ))}
    </div>
  );
}
