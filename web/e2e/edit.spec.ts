import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';

const root = () => process.env.AGENTMD_E2E_ROOT ?? '';

test('open a file through its CLAUDE.md link, edit, save, see the diff, commit and push', async ({ page }) => {
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(String(e)));
  page.on('console', (m) => {
    if (m.type() === 'error') errors.push(m.text());
  });

  await page.goto('/#/files');
  await expect(page.getByText('agent files')).toBeVisible();
  await page.getByRole('link', { name: /^instruction CLAUDE\.md/ }).click();

  const editor = page.getByRole('region', { name: 'Editor' });
  await expect(editor.getByText('Run the tests before you push.')).toBeVisible();
  await editor.locator('.cm-content').click();
  await page.keyboard.press('Control+End');
  await page.keyboard.type('\nNever push on a Friday.');
  await page.keyboard.press('Control+s');

  const changes = page.getByRole('region', { name: 'Changes' });
  await expect(changes.getByText('+Never push on a Friday.')).toBeVisible();
  await changes.getByLabel('Commit message').fill('Add a Friday rule\n\nThe e2e test adds it.');
  await changes.getByRole('button', { name: 'Commit', exact: true }).click();
  await expect(changes.getByText(/Committed [0-9a-f]{7}/)).toBeVisible();

  await changes.getByRole('button', { name: /Push to origin\/master/ }).click();
  await expect(changes.getByText('Pushed master to origin/master.')).toBeVisible();

  // The file behind the link changed, the link is still a link, and the
  // bare origin has the commit.
  const app = join(root(), 'home/code/app');
  const onDisk = execFileSync('git', ['show', 'origin/master:AGENTS.md'], { cwd: app }).toString();
  expect(onDisk).toContain('Never push on a Friday.');
  const log = execFileSync('git', ['--git-dir', join(root(), 'origin.git'), 'log', '-1', '--format=%s']).toString().trim();
  expect(log).toBe('Add a Friday rule');
  const mode = execFileSync('git', ['ls-files', '-s', 'CLAUDE.md'], { cwd: app }).toString();
  expect(mode.startsWith('120000')).toBe(true);
  expect(errors).toEqual([]);
});
