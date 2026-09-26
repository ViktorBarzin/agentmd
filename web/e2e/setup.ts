// Builds a throwaway machine (a home with a user file, one repository with a
// bare origin) and starts agentmd on it with no harness CLIs.
import { execFileSync, spawn } from 'node:child_process';
import { mkdirSync, mkdtempSync, symlinkSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';

const gitEnv = {
  ...process.env,
  GIT_AUTHOR_NAME: 'E2E',
  GIT_AUTHOR_EMAIL: 'e2e@example.com',
  GIT_COMMITTER_NAME: 'E2E',
  GIT_COMMITTER_EMAIL: 'e2e@example.com',
};

function git(cwd: string, ...args: string[]) {
  execFileSync('git', args, { cwd, env: gitEnv, stdio: 'pipe' });
}

function write(path: string, content: string) {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, content);
}

function freePort(): Promise<number> {
  return new Promise((ok, fail) => {
    const s = createServer();
    s.listen(0, '127.0.0.1', () => {
      const addr = s.address();
      const port = typeof addr === 'object' && addr ? addr.port : 0;
      s.close(() => ok(port));
    });
    s.on('error', fail);
  });
}

async function waitFor(url: string, ms: number) {
  const until = Date.now() + ms;
  while (Date.now() < until) {
    try {
      const r = await fetch(url);
      if (r.ok) return;
    } catch {
      // not up yet
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(`agentmd did not start at ${url}`);
}

export default async function setup() {
  const root = mkdtempSync(join(tmpdir(), 'agentmd-e2e-'));
  const home = join(root, 'home');
  const code = join(home, 'code');
  write(join(home, '.agents/AGENTS.md'), '# User\n\nKeep replies short.\n');
  mkdirSync(join(home, '.claude'), { recursive: true });
  symlinkSync('../.agents/AGENTS.md', join(home, '.claude/CLAUDE.md'));

  const origin = join(root, 'origin.git');
  git(root, 'init', '-q', '--bare', '-b', 'master', origin);
  const app = join(code, 'app');
  mkdirSync(code, { recursive: true });
  git(code, 'clone', '-q', origin, app);
  write(join(app, 'AGENTS.md'), '# App\n\nRun the tests before you push.\n');
  symlinkSync('AGENTS.md', join(app, 'CLAUDE.md'));
  git(app, 'add', 'AGENTS.md', 'CLAUDE.md');
  git(app, 'commit', '-q', '-m', 'init');
  git(app, 'push', '-q', 'origin', 'HEAD:master');
  git(app, 'branch', '-q', '--set-upstream-to=origin/master');

  const port = await freePort();
  const bin = process.env.AGENTMD_BIN ?? resolve('..', 'agentmd');
  const child = spawn(
    bin,
    ['serve', '--home', home, '--root', code, '--listen', `127.0.0.1:${port}`, '--config', join(root, 'none.toml')],
    {
      env: { ...gitEnv, HOME: home, AGENTMD_NO_CLIS: '1', AGENTMD_ETC: join(root, 'etc'), XDG_CACHE_HOME: join(root, 'cache') },
      stdio: 'inherit',
    },
  );
  const url = `http://127.0.0.1:${port}`;
  await waitFor(`${url}/`, 15_000);
  process.env.AGENTMD_E2E_URL = url;
  process.env.AGENTMD_E2E_ROOT = root;
  process.env.AGENTMD_E2E_PID = String(child.pid);
}
