import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';

const root = () => process.env.AGENTMD_E2E_ROOT ?? '';

// agentmd scans when it starts, which can be days before the UI opens, so
// opening the UI scans again: a file added since then shows without a rescan.
test('opening the UI shows a file added after the last scan', async ({ page }) => {
  const agents = join(root(), 'home/.claude/agents');
  mkdirSync(agents, { recursive: true });
  writeFileSync(join(agents, 'fresh-reviewer.md'), '---\nname: fresh-reviewer\ndescription: Reviews diffs.\n---\nReview the diff.\n');

  await page.goto('/#/files');
  await expect(page.getByText('agent files')).toBeVisible();
  await expect(page.getByRole('link', { name: /^subagent fresh-reviewer/ })).toBeVisible();
});
