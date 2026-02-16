import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  allowedDevOrigins: [
    "nova",
    "nova:3000",
    "localhost",
    "localhost:3000",
    "192.168.1.116",
    "192.168.1.116:3000",
  ],
  transpilePackages: ["@kandev/ui", "@kandev/theme", "@kandev/types"],
  turbopack: {},
};

export default nextConfig;
