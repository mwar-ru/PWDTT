import { describe, it, expect, beforeEach } from 'vitest';
import {
  getUpdatePauseState,
  setPauseNextUpdate,
  skipUpdateVersion,
  shouldNotifyUpdate,
} from '../lib/utils/updateSkip';

describe('updateSkip', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('notifies by default', () => {
    expect(shouldNotifyUpdate('1.6.0-st.7')).toBe(true);
  });

  it('skips a specific version until the next version arrives', () => {
    skipUpdateVersion('v1.6.0-st.7');
    expect(getUpdatePauseState().skippedVersion).toBe('1.6.0-st.7');
    expect(shouldNotifyUpdate('1.6.0-st.7')).toBe(false);
    expect(shouldNotifyUpdate('v1.6.0-st.7')).toBe(false);
    expect(shouldNotifyUpdate('1.6.0-st.8')).toBe(true);
    expect(getUpdatePauseState().skippedVersion).toBeNull();
  });

  it('pauses the next upcoming version and resumes on the one after (+2)', () => {
    setPauseNextUpdate(true);
    expect(getUpdatePauseState().pausedNext).toBe(true);

    // st.7 arrives -> silently absorbed & skipped
    expect(shouldNotifyUpdate('1.6.0-st.7')).toBe(false);
    expect(getUpdatePauseState()).toEqual({ pausedNext: false, skippedVersion: '1.6.0-st.7' });

    // Subsequent restarts while st.7 is still latest -> still silent
    expect(shouldNotifyUpdate('1.6.0-st.7')).toBe(false);

    // st.8 arrives -> notifies again
    expect(shouldNotifyUpdate('1.6.0-st.8')).toBe(true);
    expect(getUpdatePauseState()).toEqual({ pausedNext: false, skippedVersion: null });
  });

  it('clears both flags when pause is turned off', () => {
    skipUpdateVersion('1.6.0-st.7');
    setPauseNextUpdate(false);
    expect(getUpdatePauseState()).toEqual({ pausedNext: false, skippedVersion: null });
    expect(shouldNotifyUpdate('1.6.0-st.7')).toBe(true);
  });
});