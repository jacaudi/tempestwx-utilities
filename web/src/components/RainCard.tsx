import type { CurrentObservation, RainUnit } from '../types/weather';
import { PrecipitationType } from '../types/weather';
import { formatRain } from '../hooks/useUnits';
import { GlassCard } from './GlassCard';

interface RainCardProps {
  current: CurrentObservation;
  unit: RainUnit;
}

function precipLabel(type: PrecipitationType): string {
  switch (type) {
    case PrecipitationType.Rain: return 'Rain';
    case PrecipitationType.Hail: return 'Hail';
    case PrecipitationType.RainAndHail: return 'Rain & Hail';
    default: return 'None';
  }
}

export function RainCard({ current, unit }: RainCardProps) {
  const isRaining = current.precipitationType !== PrecipitationType.None;

  return (
    <GlassCard className="rain-card">
      <div className="card-header">
        <svg className="card-icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M20 17.58A5 5 0 0 0 18 8h-1.26A8 8 0 1 0 4 16.25" />
          <line x1="8" y1="16" x2="8" y2="20" />
          <line x1="12" y1="18" x2="12" y2="22" />
          <line x1="16" y1="16" x2="16" y2="20" />
        </svg>
        <span className="card-title">Precipitation</span>
        {isRaining && <span className="rain-active-badge">Active</span>}
      </div>

      <div className="rain-grid">
        <div className="rain-stat-block">
          <span className="rain-stat-label">Current</span>
          <span className="rain-stat-value">{formatRain(current.rainAccumulated, unit)}</span>
          <span className="rain-stat-type">{precipLabel(current.precipitationType)}</span>
        </div>
        <div className="rain-stat-block">
          <span className="rain-stat-label">Today</span>
          <span className="rain-stat-value">{formatRain(current.localDayRainAccumulation, unit)}</span>
        </div>
      </div>

      {isRaining && (
        <div className="rain-animation">
          {Array.from({ length: 12 }).map((_, i) => (
            <div
              key={i}
              className="raindrop"
              style={{
                left: `${8 + Math.random() * 84}%`,
                animationDelay: `${Math.random() * 1}s`,
                animationDuration: `${0.5 + Math.random() * 0.5}s`,
              }}
            />
          ))}
        </div>
      )}
    </GlassCard>
  );
}
