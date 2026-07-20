import type { UserPreferences } from '../types/weather';
import { getThemeList } from '../themes/themes';

interface SettingsPanelProps {
  isOpen: boolean;
  prefs: UserPreferences;
  onPrefsChange: (update: Partial<UserPreferences>) => void;
  onClose: () => void;
}

export function SettingsPanel({ isOpen, prefs, onPrefsChange, onClose }: SettingsPanelProps) {
  if (!isOpen) return null;

  const themeList = getThemeList();

  return (
    <div className="settings-overlay" onClick={onClose}>
      <div className="settings-panel glass-card" onClick={(e) => e.stopPropagation()}>
        <div className="settings-header">
          <h2>Settings</h2>
          <button className="settings-close" onClick={onClose} aria-label="Close settings">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
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

        <div className="settings-section">
          <h3>API Configuration</h3>
          <div className="setting-row">
            <label>Station ID</label>
            <input
              type="text"
              className="glass-input"
              placeholder="e.g. 12345"
              defaultValue=""
            />
          </div>
          <div className="setting-row">
            <label>API Token</label>
            <input
              type="password"
              className="glass-input"
              placeholder="Personal Access Token"
              defaultValue=""
            />
          </div>
          <p className="settings-hint">
            Get your token at{' '}
            <a href="https://tempestwx.com" target="_blank" rel="noopener noreferrer">
              tempestwx.com
            </a>{' '}
            → Settings → Data Authorizations
          </p>
        </div>
      </div>
    </div>
  );
}
