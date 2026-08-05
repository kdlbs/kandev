import { Component, type ErrorInfo, type ReactNode } from "react";
import { usePathname, useSearchParams } from "@/lib/routing/client-router";
import { useTranslation } from "react-i18next";

type AppErrorBoundaryProps = {
  scope: "root" | "route";
  resetKey?: string;
  children: ReactNode;
};

type AppErrorBoundaryState = {
  error: Error | null;
};

class AppErrorBoundary extends Component<AppErrorBoundaryProps, AppErrorBoundaryState> {
  state: AppErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): AppErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error(`[app] ${this.props.scope} render failed`, error, info.componentStack);
  }

  componentDidUpdate(previousProps: AppErrorBoundaryProps): void {
    if (this.state.error && previousProps.resetKey !== this.props.resetKey) {
      this.setState({ error: null });
    }
  }

  render(): ReactNode {
    if (!this.state.error) return this.props.children;
    return this.props.scope === "root" ? <RootErrorFallback /> : <RouteErrorFallback />;
  }
}

function RootErrorFallback() {
  const { t } = useTranslation();
  return (
    <div
      role="alert"
      className="flex min-h-dvh w-full items-center justify-center bg-background p-6 text-foreground"
    >
      <ErrorRecoveryContent
        title={t("common:kandevCouldNotLoad")}
        description={t("common:reloadTheApplicationToTryAgain")}
      />
    </div>
  );
}

function RouteErrorFallback() {
  const { t } = useTranslation();
  return (
    <div
      role="alert"
      className="flex h-full min-h-0 w-full items-center justify-center p-6 text-foreground"
    >
      <ErrorRecoveryContent
        title={t("common:thisPageCouldNotLoad")}
        description={t("common:youCanUseTheNavigationOr")}
      />
    </div>
  );
}

function ErrorRecoveryContent({ title, description }: { title: string; description: string }) {
  const { t } = useTranslation();
  return (
    <div className="flex max-w-sm flex-col items-center gap-3 text-center">
      <p className="text-base font-medium">{title}</p>
      <p className="text-sm text-muted-foreground">{description}</p>
      <button
        type="button"
        className="min-h-11 cursor-pointer rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground"
        onClick={() => window.location.reload()}
      >
        {t("common:reload")}
      </button>
    </div>
  );
}

export function RootErrorBoundary({ children }: { children: ReactNode }) {
  return <AppErrorBoundary scope="root">{children}</AppErrorBoundary>;
}

export function RouteErrorBoundary({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const search = typeof window === "undefined" ? searchParams.toString() : window.location.search;
  const resetKey = `${pathname}\n${search}`;

  return (
    <AppErrorBoundary scope="route" resetKey={resetKey}>
      {children}
    </AppErrorBoundary>
  );
}
