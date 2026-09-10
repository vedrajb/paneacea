import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./e2e",
  outputDir: "../.build-validation/browser-results",
  timeout: 30000,
  use: {
    baseURL: "http://127.0.0.1:5174",
    browserName: "chromium",
    channel: "msedge",
    headless: true,
    viewport: { width: 1400, height: 900 },
  },
  webServer: {
    command: "node ./node_modules/vite/bin/vite.js --host 127.0.0.1 --port 5174",
    url: "http://127.0.0.1:5174",
    reuseExistingServer: false,
  },
});
