import path from "node:path";

import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // The dependencies live in the repository root (one node_modules for both the
  // web app and the worker), so file tracing has to start there.
  outputFileTracingRoot: path.resolve(__dirname, ".."),
};

export default nextConfig;
