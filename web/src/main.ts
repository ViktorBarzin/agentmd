import { mount } from 'svelte';
import './app.css';
import App from './App.svelte';
import { theme } from './lib/theme.svelte';

async function start() {
  // The mock is a separate chunk that only `VITE_MOCK=1 npm run dev` loads.
  if (import.meta.env.VITE_MOCK === '1') {
    const { installMock } = await import('./lib/mock/install');
    installMock();
  }
  theme.init();
  const target = document.getElementById('app');
  if (!target) throw new Error('The page has no #app element.');
  mount(App, { target });
}

void start();
