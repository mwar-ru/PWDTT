const KEY = 'pwdtt_theme';

export type Theme = 'light' | 'dark' | 'auto';
export type ResolvedTheme = 'light' | 'dark';

type Listener = (t: Theme) => void;

function isValidTheme(v: string | null): v is Theme {
  return v === 'light' || v === 'dark' || v === 'auto';
}

function getSystemTheme(): ResolvedTheme {
  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }
  return 'light';
}

function resolveTheme(t: Theme): ResolvedTheme {
  return t === 'auto' ? getSystemTheme() : t;
}

const stored = localStorage.getItem(KEY);
let current: Theme = isValidTheme(stored) ? stored : 'auto';
const listeners = new Set<Listener>();

function apply(t: Theme) {
  const resolved = resolveTheme(t);
  document.documentElement.setAttribute('data-theme', resolved);
  document.documentElement.style.colorScheme = resolved;
}

apply(current);

if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
  const mql = window.matchMedia('(prefers-color-scheme: dark)');
  const onSystemChange = () => {
    if (current === 'auto') {
      apply('auto');
      listeners.forEach(fn => fn(current));
    }
  };
  if (typeof mql.addEventListener === 'function') {
    mql.addEventListener('change', onSystemChange);
  } else if (typeof mql.addListener === 'function') {
    mql.addListener(onSystemChange);
  }
}

export const themeStore = {
  get: () => current,
  getResolved: (): ResolvedTheme => resolveTheme(current),
  set: (t: Theme) => {
    current = t;
    localStorage.setItem(KEY, t);
    apply(t);
    listeners.forEach(fn => fn(t));
  },
  toggle: () => themeStore.set(resolveTheme(current) === 'light' ? 'dark' : 'light'),
  cycle: () => {
    const next: Theme = current === 'light' ? 'dark' : current === 'dark' ? 'auto' : 'light';
    themeStore.set(next);
  },
  subscribe: (fn: Listener) => {
    listeners.add(fn);
    fn(current);
    return () => { listeners.delete(fn); };
  },
};