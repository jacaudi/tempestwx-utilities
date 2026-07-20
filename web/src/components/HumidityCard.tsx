import type { CurrentObservation, TemperatureUnit } from '../types/weather';
import { formatTemp } from '../hooks/useUnits';
import { GlassCard } from './GlassCard';

interface HumidityCardProps {
  current: CurrentObservation;
  tempUnit: TemperatureUnit;
}

function humidityLevel(rh: number): string {
  if (rh < 30) return 'Dry';
  if (rh < 50) return 'Comfortable';
  if (rh < 70) return 'Humid';
  return 'Very Humid';
}

export function HumidityCard({ current, tempUnit }: HumidityCardProps) {
  const pct = current.relativeHumidity;
  const circumference = 2 * Math.PI * 40;
  const offset = circumference - (pct / 100) * circumference;

  return (
    <GlassCard className="humidity-card">
      <div className="card-header">
        <svg className="card-icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M12 2.69l5.66 5.66a8 8 0 1 1-11.31 0z" />
        </svg>
        <span className="card-title">Humidity</span>
      </div>

      <div className="humidity-ring-container">
        <svg className="humidity-ring" viewBox="0 0 100 100">
          <circle className="ring-bg" cx="50" cy="50" r="40" />
          <circle
            className="ring-fill"
            cx="50"
            cy="50"
            r="40"
            strokeDasharray={circumference}
            strokeDashoffset={offset}
            transform="rotate(-90 50 50)"
          />
        </svg>
        <div className="humidity-ring-text">
          <span className="humidity-value">{Math.round(pct)}%</span>
          <span className="humidity-level">{humidityLevel(pct)}</span>
        </div>
      </div>

      <div className="humidity-stats-row">
        <div className="humidity-stat">
          <span className="humidity-stat-label">Dew Point</span>
          <span className="humidity-stat-value">{formatTemp(current.dewPoint, tempUnit)}</span>
        </div>
        <div className="humidity-stat">
          <span className="humidity-stat-label">Wet Bulb</span>
          <span className="humidity-stat-value">{formatTemp(current.wetBulbTemperature, tempUnit)}</span>
        </div>
      </div>
    </GlassCard>
  );
}
