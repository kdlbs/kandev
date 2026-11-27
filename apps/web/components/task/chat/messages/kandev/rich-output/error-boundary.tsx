import { Component, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

type BoundaryProps = { children: ReactNode; resetKey: string; fallback: ReactNode };
class NativeContentBoundary extends Component<BoundaryProps, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  componentDidUpdate(previous: BoundaryProps) {
    if (this.state.failed && previous.resetKey !== this.props.resetKey)
      this.setState({ failed: false });
  }
  render() {
    return this.state.failed ? this.props.fallback : this.props.children;
  }
}
export function RichOutputContentBoundary({ children, resetKey }: Omit<BoundaryProps, "fallback">) {
  const { t } = useTranslation();
  return (
    <NativeContentBoundary
      resetKey={resetKey}
      fallback={<p className="text-xs text-muted-foreground">{t("task:richOutputUnavailable")}</p>}
    >
      {children}
    </NativeContentBoundary>
  );
}
