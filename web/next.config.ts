import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";
import withSerwistInit from "@serwist/next";

const withNextIntl = createNextIntlPlugin("./src/lib/i18n/request.ts");

const withSerwist = withSerwistInit({
  swSrc: "src/app/sw.ts",
  swDest: "public/sw.js",
  // The service worker is only useful for the installed app shell; keep dev
  // and Playwright runs deterministic.
  disable: process.env.NODE_ENV !== "production",
  cacheOnNavigation: true,
  reloadOnOnline: true,
  // The public receipt is a document, not part of the app shell. Never cache it.
  exclude: [/^r\//, /\.map$/, /^manifest.*\.js$/],
});

const nextConfig: NextConfig = {
  reactStrictMode: true,
  poweredByHeader: false,
  output: "standalone",
  // Server components and route handlers talk to the API over the compose
  // network; the browser talks to it over the published port.
  env: {
    NEXT_PUBLIC_API_BASE_URL: process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080",
  },
  headers: async () => [
    {
      source: "/(.*)",
      headers: [
        { key: "X-Content-Type-Options", value: "nosniff" },
        { key: "X-Frame-Options", value: "DENY" },
        { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
        { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=()" },
      ],
    },
  ],
};

export default withSerwist(withNextIntl(nextConfig));
