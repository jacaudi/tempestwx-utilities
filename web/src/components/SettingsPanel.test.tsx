import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { SettingsPanel } from './SettingsPanel';
import type { UserPreferences } from '../types/weather';

const prefs: UserPreferences = {
  temperatureUnit: 'F',
  windUnit: 'mph',
  pressureUnit: 'inHg',
  rainUnit: 'in',
  theme: 'liquid-glass',
};

describe('SettingsPanel', () => {
  it('does not render the dead Station ID / API Token inputs (token is server-side now)', () => {
    render(
      <SettingsPanel
        isOpen={true}
        prefs={prefs}
        onPrefsChange={vi.fn()}
        onClose={vi.fn()}
      />
    );

    expect(screen.queryByText(/station id/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/api token/i)).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText(/personal access token/i)).not.toBeInTheDocument();
  });

  it('still renders the real settings (units and theme)', () => {
    render(
      <SettingsPanel
        isOpen={true}
        prefs={prefs}
        onPrefsChange={vi.fn()}
        onClose={vi.fn()}
      />
    );

    expect(screen.getByText('Temperature')).toBeInTheDocument();
    expect(screen.getByText('°F')).toBeInTheDocument();
    expect(screen.getByText('Theme')).toBeInTheDocument();
  });
});
