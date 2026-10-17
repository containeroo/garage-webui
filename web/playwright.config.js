import { defineConfig } from "@playwright/test";

if (!process.env.E2E_BASE_URL) {
  throw new Error(
    "Set E2E_BASE_URL to an isolated, fresh test instance; never use production.",
  );
}

export default defineConfig({
  testDir: "./e2e",
  timeout: 120_000,
  expect: { timeout: 10_000 },
  workers: 1,
  fullyParallel: false,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL: process.env.E2E_BASE_URL.replace(/\/?$/, "/"),
    trace: "retain-on-failure",
    browserName: "chromium",
    launchOptions: process.env.E2E_CHROMIUM_PATH
      ? { executablePath: process.env.E2E_CHROMIUM_PATH }
      : {},
  },
});
