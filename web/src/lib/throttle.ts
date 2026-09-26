/** Lets an action run at most once per interval, such as a rescan on focus. */
export function createThrottle(intervalMs: number, now: () => number = Date.now) {
  let last = Number.NEGATIVE_INFINITY;
  return {
    /** Records a run and returns true when the interval has passed. */
    tryRun(): boolean {
      const t = now();
      if (t - last < intervalMs) return false;
      last = t;
      return true;
    },
    /** Records a run that happened some other way. */
    mark(): void {
      last = now();
    },
  };
}
