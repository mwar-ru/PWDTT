const SKIPPED_VERSION_KEY = 'pwdtt_skipped_update_version';
const PAUSE_NEXT_KEY = 'pwdtt_pause_next_update';

export interface UpdatePauseState {
  pausedNext: boolean;
  skippedVersion: string | null;
}

function normalizeVer(v: string): string {
  return v.trim().replace(/^v/i, '');
}

export function getUpdatePauseState(): UpdatePauseState {
  try {
    const pausedNext = localStorage.getItem(PAUSE_NEXT_KEY) === '1';
    const rawSkipped = localStorage.getItem(SKIPPED_VERSION_KEY);
    const skippedVersion = rawSkipped ? normalizeVer(rawSkipped) : null;
    return { pausedNext, skippedVersion };
  } catch {
    return { pausedNext: false, skippedVersion: null };
  }
}

export function setPauseNextUpdate(paused: boolean): void {
  try {
    if (paused) {
      localStorage.setItem(PAUSE_NEXT_KEY, '1');
      localStorage.removeItem(SKIPPED_VERSION_KEY);
    } else {
      localStorage.removeItem(PAUSE_NEXT_KEY);
      localStorage.removeItem(SKIPPED_VERSION_KEY);
    }
  } catch {
    // ignore storage errors
  }
}

export function skipUpdateVersion(version: string): void {
  const norm = normalizeVer(version);
  if (!norm) return;
  try {
    localStorage.setItem(SKIPPED_VERSION_KEY, norm);
    localStorage.removeItem(PAUSE_NEXT_KEY);
  } catch {
    // ignore storage errors
  }
}

// Возвращает true, если нужно показать окно обновления для latestVersion.
// Если включена пауза ("приостановить обновления"), первая новая версия
// автоматически запоминается как пропущенная, а уведомление вернётся только
// на следующей (+1) версии после неё.
export function shouldNotifyUpdate(latestVersion: string): boolean {
  const norm = normalizeVer(latestVersion);
  if (!norm) return false;
  try {
    if (localStorage.getItem(PAUSE_NEXT_KEY) === '1') {
      localStorage.setItem(SKIPPED_VERSION_KEY, norm);
      localStorage.removeItem(PAUSE_NEXT_KEY);
      return false;
    }
    const skipped = localStorage.getItem(SKIPPED_VERSION_KEY);
    if (skipped) {
      if (normalizeVer(skipped) === norm) {
        return false;
      }
      localStorage.removeItem(SKIPPED_VERSION_KEY);
    }
  } catch {
    return true;
  }
  return true;
}