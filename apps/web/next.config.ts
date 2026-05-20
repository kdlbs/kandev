import type { NextConfig } from "next";

// The kandev CLI auto-populates NEXT_ALLOWED_DEV_ORIGINS with the host's
// non-loopback addresses (see apps/cli/src/shared.ts) so LAN / Tailscale /
// SSH-forwarded clients pass Next.js's allowedDevOrigins check. Users running
// `next dev` directly can still extend the list manually via that env var.
const defaultDevOrigins = ["localhost", "localhost:37429"];
const extraDevOrigins = process.env.NEXT_ALLOWED_DEV_ORIGINS
  ? process.env.NEXT_ALLOWED_DEV_ORIGINS.split(",")
      .map((s) => s.trim())
      .filter(Boolean)
  : [];

const nextConfig: NextConfig = {
  output: "standalone",
  allowedDevOrigins: [...defaultDevOrigins, ...extraDevOrigins],
  transpilePackages: ["@kandev/ui", "@kandev/theme", "@kandev/types"],
  turbopack: {},
};

export default nextConfig;
