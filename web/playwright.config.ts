import { defineConfig } from '@playwright/test';

// Drives the built binary (../agentmd, or AGENTMD_BIN) against a throwaway
// home and repository made by e2e/setup.ts. Set AGENTMD_E2E_CHROME to use an
// installed Chrome instead of Playwright's own browser.
export default defineConfig({
  testDir: 'e2e',
  timeout: 60_000,
  retries: 0,
  workers: 1,
  globalSetup: './e2e/setup.ts',
  globalTeardown: './e2e/teardown.ts',
  reporter: [['list']],
  use: {
    baseURL: process.env.AGENTMD_E2E_URL,
    viewport: { width: 1440, height: 900 },
    trace: 'retain-on-failure',
    launchOptions: process.env.AGENTMD_E2E_CHROME ? { executablePath: process.env.AGENTMD_E2E_CHROME } : {},
  },
});
