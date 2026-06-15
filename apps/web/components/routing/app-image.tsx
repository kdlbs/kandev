"use client";

import type { ImgHTMLAttributes } from "react";

export type AppImageProps = Omit<ImgHTMLAttributes<HTMLImageElement>, "src" | "alt"> & {
  src: string;
  alt: string;
  unoptimized?: boolean;
  priority?: boolean;
};

export default function Image({
  alt,
  unoptimized: _unoptimized,
  priority: _priority,
  ...props
}: AppImageProps) {
  // eslint-disable-next-line @next/next/no-img-element -- SPA image adapter intentionally renders a native image.
  return <img alt={alt} {...props} />;
}
