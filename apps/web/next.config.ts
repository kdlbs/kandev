import type { NextConfig } from "next";

const defaultDevOrigins = [
  "localhost",
  "localhost:3000",
  "192.168.1.116", // Maintainer dev server
  "100.105.155.17", // Maintainer dev server (Tailscale)
];
const extraDevOrigins = process.env.NEXT_ALLOWED_DEV_ORIGINS
  ? process.env.NEXT_ALLOWED_DEV_ORIGINS.split(",").map((s) => s.trim())
  : [];

const nextConfig: NextConfig = {
  output: "standalone",
  allowedDevOrigins: [...defaultDevOrigins, ...extraDevOrigins],
  transpilePackages: ["@kandev/ui", "@kandev/theme", "@kandev/types"],
  turbopack: {},
};

export default nextConfig;
