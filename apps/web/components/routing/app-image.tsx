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
  return <img alt={alt} {...props} />;
}
