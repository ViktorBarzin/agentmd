import { describe, expect, it } from 'vitest';
import { createThrottle } from './throttle';

describe('createThrottle', () => {
  it('runs at most once per interval', () => {
    let t = 1_000_000;
    const th = createThrottle(10_000, () => t);
    expect(th.tryRun()).toBe(true);
    t += 3_000;
    expect(th.tryRun()).toBe(false);
    t += 6_999;
    expect(th.tryRun()).toBe(false);
    t += 1;
    expect(th.tryRun()).toBe(true);
  });

  it('counts a manual run as a run', () => {
    let t = 0;
    const th = createThrottle(10_000, () => t);
    th.mark();
    t = 9_000;
    expect(th.tryRun()).toBe(false);
    t = 10_000;
    expect(th.tryRun()).toBe(true);
  });
});
