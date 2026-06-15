import { Suspense, createElement, lazy, type ComponentType, type ReactNode } from "react";

type DynamicModule<P> = { default: ComponentType<P> } | ComponentType<P>;

type DynamicOptions = {
  loading?: () => ReactNode;
  ssr?: boolean;
};

export default function dynamic<P>(
  loader: () => Promise<DynamicModule<P>>,
  options: DynamicOptions = {},
): ComponentType<P> {
  const LazyComponent = lazy(async () => {
    const mod = await loader();
    return "default" in mod ? mod : { default: mod };
  });

  const DynamicComponent = (props: P) =>
    createElement(
      Suspense,
      { fallback: options.loading?.() ?? null },
      createElement(LazyComponent as ComponentType<Record<string, unknown>>, {
        ...(props as Record<string, unknown>),
      }),
    );
  DynamicComponent.displayName = "DynamicComponent";
  return DynamicComponent;
}
