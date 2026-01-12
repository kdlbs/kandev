import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  transpilePackages: ["@kandev/ui", "@kandev/theme", "@kandev/types"],
};

export default nextConfig;
