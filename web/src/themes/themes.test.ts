import { describe, it, expect, afterEach } from 'vitest';
import { applyTheme, themes } from './themes';

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

  it('removes a synthetic prior var value the incoming theme does not redefine', () => {
    document.documentElement.style.setProperty('--text-shadow', 'LEAKED');

    applyTheme('desert-sunset');

    expect(document.documentElement.style.getPropertyValue('--text-shadow')).toBe(
      themes['desert-sunset'].vars['--text-shadow']
    );
    expect(document.documentElement.style.getPropertyValue('--text-shadow')).not.toBe('LEAKED');
  });

  it('every theme defines --text-shadow', () => {
    for (const theme of Object.values(themes)) {
      expect(theme.vars['--text-shadow']).toBeTruthy();
    }
  });
});
