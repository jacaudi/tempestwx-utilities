import { describe, it, expect, afterEach } from 'vitest';
import { applyTheme, themes, getThemeList } from './themes';

afterEach(() => {
  // Reset any inline custom properties the tests apply, so themes.ts's
  // own leak fix (not a leftover from a previous test) is what's asserted.
  document.documentElement.removeAttribute('style');
});

describe('applyTheme (P2.15 — theme-var leak)', () => {
  it('does not leak a previous theme value: nord then desert-sunset ends with desert-sunset\'s --text-shadow', () => {
    applyTheme('nord');
    expect(document.documentElement.style.getPropertyValue('--text-shadow')).toBe(
      themes.nord.vars['--text-shadow']
    );

    applyTheme('desert-sunset');

    expect(document.documentElement.style.getPropertyValue('--text-shadow')).toBe(
      themes['desert-sunset'].vars['--text-shadow']
    );
    expect(document.documentElement.style.getPropertyValue('--text-shadow')).not.toBe(
      themes.nord.vars['--text-shadow']
    );
  });

  it('overwrites a leaked value when the incoming theme redefines the var', () => {
    // A stale value for a var the incoming theme DOES define is corrected by
    // the setProperty pass (the removeProperty pass never fires for it, since
    // every current theme defines the full var set).
    document.documentElement.style.setProperty('--text-shadow', 'LEAKED');

    applyTheme('desert-sunset');

    expect(document.documentElement.style.getPropertyValue('--text-shadow')).toBe(
      themes['desert-sunset'].vars['--text-shadow']
    );
    expect(document.documentElement.style.getPropertyValue('--text-shadow')).not.toBe('LEAKED');
  });

  it('clears only theme-owned vars, leaving unrelated inline custom properties intact', () => {
    // The leak-clearing pass must scope its removeProperty calls to the union
    // of theme vars — it must NOT nuke arbitrary inline custom properties set
    // by other code. (This exercises the removeProperty pass's key guard.)
    document.documentElement.style.setProperty('--not-a-theme-var', 'KEEP');

    applyTheme('nord');

    expect(document.documentElement.style.getPropertyValue('--not-a-theme-var')).toBe('KEEP');
  });

  it('every theme defines --text-shadow', () => {
    for (const theme of Object.values(themes)) {
      expect(theme.vars['--text-shadow']).toBeTruthy();
    }
  });
});

describe('getThemeList (§14 — theme-swatch backgroundImage wiring)', () => {
  it('includes a truthy backgroundImage for every entry, matching the source theme', () => {
    const list = getThemeList();
    expect(list.length).toBe(Object.keys(themes).length);
    for (const entry of list) {
      expect(entry.backgroundImage).toBeTruthy();
      expect(entry.backgroundImage).toBe(themes[entry.name].backgroundImage);
    }
  });
});
