import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  serverExternalPackages: ['docx-preview', 'docx'],
  output: 'standalone',
  eslint: {
    // Allow builds to proceed even if lint warnings or errors are present.
    ignoreDuringBuilds: true,
  },
};

export default nextConfig;
