import { rmSync } from 'node:fs';

export default async function teardown() {
  const pid = Number(process.env.AGENTMD_E2E_PID);
  if (pid) {
    try {
      process.kill(pid);
    } catch {
      // already gone
    }
  }
  if (process.env.AGENTMD_E2E_ROOT) rmSync(process.env.AGENTMD_E2E_ROOT, { recursive: true, force: true });
}
