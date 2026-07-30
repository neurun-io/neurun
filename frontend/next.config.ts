import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Emit the minimal traced Node.js server used by the production container.
  output: "standalone",
};

export default nextConfig;
