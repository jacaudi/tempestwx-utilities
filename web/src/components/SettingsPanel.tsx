import { useEffect, useRef } from 'react';
import type { UserPreferences } from '../types/weather';
import { getThemeList } from '../themes/themes';

interface SettingsPanelProps {
  isOpen: boolean;
  prefs: UserPreferences;
  onPrefsChange: (update: Partial<UserPreferences>) => void;
  onClose: () => void;
}

const SETTINGS_TITLE_ID = 'settings-title';
const FOCUSABLE_SELECTOR =
  'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';

export function SettingsPanel({ isOpen, prefs, onPrefsChange, onClose }: SettingsPanelProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const previouslyFocusedRef = useRef<HTMLElement | null>(null);

  // Focus management (P2.14): on open, remember what was focused and move
  // focus into the dialog; the cleanup (fired when isOpen flips to false,
  // or on unmount) restores focus to whatever triggered the dialog.
  useEffect(() => {
    if (!isOpen) return;
    previouslyFocusedRef.current = document.activeElement as HTMLElement | null;
    dialogRef.current?.focus();
    return () => {
      previouslyFocusedRef.current?.focus();
    };
  }, [isOpen]);

  if (!isOpen) return null;

  const themeList = getThemeList();

  function handleKeyDown(e: React.KeyboardEvent<HTMLDivElement>) {
    if (e.key === 'Escape') {
      onClose();
      return;
    }
    if (e.key !== 'Tab') return;

    const dialog = dialogRef.current;
    if (!dialog) return;
    const focusable = dialog.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR);
    if (focusable.length === 0) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];

    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }

  return (
    <div className="settings-overlay" onClick={onClose}>
      <div
        className="settings-panel glass-card"
        onClick={(e) => e.stopPropagation()}
        onKeyDown={handleKeyDown}
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={SETTINGS_TITLE_ID}
        tabIndex={-1}
      >
        <div className="settings-header">
          <h2 id={SETTINGS_TITLE_ID}>Settings</h2>
          <button className="settings-close" onClick={onClose} aria-label="Close settings">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>

        <div className="settings-section">
          <h3>Units</h3>
          <div className="setting-row">
            <label>Temperature</label>
            <div className="toggle-group">
              <button
                className={prefs.temperatureUnit === 'F' ? 'active' : ''}
                onClick={() => onPrefsChange({ temperatureUnit: 'F' })}
              >°F</button>
              <button
                className={prefs.temperatureUnit === 'C' ? 'active' : ''}
                onClick={() => onPrefsChange({ temperatureUnit: 'C' })}
              >°C</button>
            </div>
          </div>
          <div className="setting-row">
            <label>Wind Speed</label>
            <div className="toggle-group">
              {(['mph', 'kph', 'ms', 'kts'] as const).map((u) => (
                <button
                  key={u}
                  className={prefs.windUnit === u ? 'active' : ''}
                  onClick={() => onPrefsChange({ windUnit: u })}
                >{u}</button>
              ))}
            </div>
          </div>
          <div className="setting-row">
            <label>Pressure</label>
            <div className="toggle-group">
              {(['inHg', 'mb', 'hPa'] as const).map((u) => (
                <button
                  key={u}
                  className={prefs.pressureUnit === u ? 'active' : ''}
                  onClick={() => onPrefsChange({ pressureUnit: u })}
                >{u}</button>
              ))}
            </div>
          </div>
          <div className="setting-row">
            <label>Rainfall</label>
            <div className="toggle-group">
              <button
                className={prefs.rainUnit === 'in' ? 'active' : ''}
                onClick={() => onPrefsChange({ rainUnit: 'in' })}
              >in</button>
              <button
                className={prefs.rainUnit === 'mm' ? 'active' : ''}
                onClick={() => onPrefsChange({ rainUnit: 'mm' })}
              >mm</button>
            </div>
          </div>
        </div>

        <div className="settings-section">
          <h3>Theme</h3>
          <div className="theme-grid">
            {themeList.map((t) => (
              <button
                key={t.name}
                className={`theme-option ${prefs.theme === t.name ? 'active' : ''}`}
                onClick={() => onPrefsChange({ theme: t.name })}
              >
                <span className="theme-swatch" data-theme={t.name} />
                <span className="theme-name">{t.label}</span>
                <span className="theme-desc">{t.description}</span>
              </button>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
