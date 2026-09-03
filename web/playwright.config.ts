import { defineConfig, devices } from "@playwright/test";

/**
 * Smoke tests. By default (`make e2e`) they start a stub API on :18080 and the
 * built Next app on :3100 so the suite is self-contained. Set
 * PLAYWRIGHT_BASE_URL to point at a running stack instead (no servers started).
 */
const external = process.env.PLAYWRIGHT_BASE_URL;
const port = Number(process.env.PLAYWRIGHT_PORT ?? 3100);
const stubPort = Number(process.env.STUB_PORT ?? 18080);
const baseURL = external ?? `http://localhost:${port}`;
const apiURL = `http://127.0.0.1:${stubPort}`;

export default defineConfig({
  testDir: "./e2e",
  timeout: 30_000,
  fullyParallel: true,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["github"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL,
    trace: "retain-on-failure",
  },
  webServer: external
    ? undefined
    : [
        {
          command: "node e2e/stub-api.mjs",
          url: `${apiURL}/healthz`,
          reuseExistingServer: false,
          env: { STUB_PORT: String(stubPort), WEB_ORIGIN: baseURL },
        },
        {
          command: `npx next start -p ${port}`,
          url: `${baseURL}/manifest.webmanifest`,
          reuseExistingServer: false,
          timeout: 60_000,
          env: {
            NEXT_PUBLIC_API_BASE_URL: apiURL,
            API_BASE_URL: apiURL,
            NEXT_PUBLIC_WEB_BASE_URL: baseURL,
          },
        },
      ],
  projects: [
    { name: "mobile", use: { ...devices["Pixel 5"] } },
    { name: "desktop", use: { ...devices["Desktop Chrome"] } },
  ],
});
