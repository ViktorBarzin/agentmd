// Light, dark, or follow the system. The choice is kept in localStorage.
import { readStored, writeStored } from './storage';

export type ThemeMode = 'system' | 'light' | 'dark';

const KEY = 'agentmd.theme';

function readMode(): ThemeMode {
  const v = readStored(KEY);
  return v === 'light' || v === 'dark' ? v : 'system';
}

function apply(mode: ThemeMode) {
  const root = document.documentElement;
  if (mode === 'system') delete root.dataset.theme;
  else root.dataset.theme = mode;
}

class Theme {
  mode = $state<ThemeMode>('system');
  systemDark = $state(false);
  effective = $derived<'light' | 'dark'>(this.mode === 'system' ? (this.systemDark ? 'dark' : 'light') : this.mode);

  /** Applies the stored choice and follows system changes. Call once at start. */
  init() {
    this.mode = readMode();
    apply(this.mode);
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    this.systemDark = mq.matches;
    mq.addEventListener('change', (e) => {
      this.systemDark = e.matches;
    });
  }

  set(mode: ThemeMode) {
    this.mode = mode;
    apply(mode);
    writeStored(KEY, mode === 'system' ? null : mode);
  }

  /** system, then light, then dark. */
  cycle() {
    this.set(this.mode === 'system' ? 'light' : this.mode === 'light' ? 'dark' : 'system');
  }
}

export const theme = new Theme();
