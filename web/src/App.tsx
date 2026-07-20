import { useState, useEffect } from 'react';
import { useWeatherData } from './hooks/useWeatherData';
import { useUnits } from './hooks/useUnits';
import { applyTheme } from './themes/themes';
import { Header } from './components/Header';
import { TemperatureHero } from './components/TemperatureHero';
import { WindCard } from './components/WindCard';
import { PressureCard } from './components/PressureCard';
import { HumidityCard } from './components/HumidityCard';
import { SolarUVCard } from './components/SolarUVCard';
import { RainCard } from './components/RainCard';
import { LightningCard } from './components/LightningCard';
import { ForecastStrip } from './components/ForecastStrip';
import { StationHealth } from './components/StationHealth';
import { AlmanacCard } from './components/AlmanacCard';
import { SettingsPanel } from './components/SettingsPanel';
import type { ThemeName } from './types/weather';
import './App.css';

function App() {
  const { station, current, forecast, status, almanac, isLoading, error, lastUpdated, refresh } =
    useWeatherData();
  const { prefs, setPrefs } = useUnits();
  const [settingsOpen, setSettingsOpen] = useState(false);

  useEffect(() => {
    applyTheme(prefs.theme as ThemeName);
  }, [prefs.theme]);

  if (isLoading) {
    return (
      <div className="loading-screen">
        <div className="loading-spinner" />
        <p>Connecting to station...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="error-screen">
        <h2>Connection Error</h2>
        <p>{error}</p>
        <button className="glass-btn" onClick={refresh}>Retry</button>
      </div>
    );
  }

  if (!current) return null;

  return (
    <div className="app">
      <div className="app-bg-orbs">
        <div className="orb orb-1" />
        <div className="orb orb-2" />
        <div className="orb orb-3" />
      </div>

      <Header
        station={station}
        status={status}
        lastUpdated={lastUpdated}
        onSettingsClick={() => setSettingsOpen(true)}
      />

      <main className="dashboard">
        <div className="dashboard-grid">
          <TemperatureHero current={current} unit={prefs.temperatureUnit} precipProbability={forecast[0]?.precipProbability} />
          <WindCard current={current} unit={prefs.windUnit} />
          <HumidityCard current={current} tempUnit={prefs.temperatureUnit} />
          <PressureCard current={current} unit={prefs.pressureUnit} />
          <SolarUVCard current={current} />
          <RainCard current={current} unit={prefs.rainUnit} />
          <LightningCard current={current} />
          {status && <StationHealth status={status} />}
          <ForecastStrip forecast={forecast} unit={prefs.temperatureUnit} />
          {almanac && <AlmanacCard almanac={almanac} unit={prefs.temperatureUnit} />}
        </div>
      </main>

      <SettingsPanel
        isOpen={settingsOpen}
        prefs={prefs}
        onPrefsChange={setPrefs}
        onClose={() => setSettingsOpen(false)}
      />
    </div>
  );
}

export default App;
