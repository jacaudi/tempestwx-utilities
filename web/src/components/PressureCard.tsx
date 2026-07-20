import type { CurrentObservation, PressureUnit } from '../types/weather';
import { formatPressure } from '../hooks/useUnits';
import { PressureTrend } from '../types/weather';
import { GlassCard } from './GlassCard';

interface PressureCardProps {
  current: CurrentObservation;
  unit: PressureUnit;
}

function trendArrow(trend: PressureTrend): string {
  switch (trend) {
    case PressureTrend.Rising: return '↑';
    case PressureTrend.Falling: return '↓';
    default: return '→';
  }
}

function trendLabel(trend: PressureTrend): string {
  switch (trend) {
    case PressureTrend.Rising: return 'Rising';
    case PressureTrend.Falling: return 'Falling';
    default: return 'Steady';
  }
}

export function PressureCard({ current, unit }: PressureCardProps) {
  return (
    <GlassCard className="pressure-card">
      <div className="card-header">
        <svg className="card-icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <circle cx="12" cy="12" r="10" />
          <path d="M12 6v6l4 2" />
        </svg>
        <span className="card-title">Pressure</span>
      </div>
      <div className="pressure-value-block">
        <span className="pressure-value">{formatPressure(current.stationPressure, unit)}</span>
        <span className={`pressure-trend trend-${current.pressureTrend}`}>
          {trendArrow(current.pressureTrend)} {trendLabel(current.pressureTrend)}
        </span>
      </div>
      <div className="pressure-gauge">
        <div className="gauge-track">
          <div
            className="gauge-fill"
            style={{
              width: `${Math.min(100, Math.max(0, ((current.stationPressure - 980) / 60) * 100))}%`,
            }}
          />
        </div>
        <div className="gauge-labels">
          <span>Low</span>
          <span>Normal</span>
          <span>High</span>
        </div>
      </div>
    </GlassCard>
  );
}
