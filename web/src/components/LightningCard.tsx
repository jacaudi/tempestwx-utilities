import type { CurrentObservation } from '../types/weather';
import { GlassCard } from './GlassCard';

interface LightningCardProps {
  current: CurrentObservation;
}

export function LightningCard({ current }: LightningCardProps) {
  const hasStrikes = current.lightningStrikeCount > 0;

  return (
    <GlassCard className={`lightning-card ${hasStrikes ? 'lightning-active' : ''}`}>
      <div className="card-header">
        <svg className="card-icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
        </svg>
        <span className="card-title">Lightning</span>
        {hasStrikes && <span className="lightning-alert-badge">Detected</span>}
      </div>

      <div className="lightning-content">
        <div className="lightning-stats-row">
          <div className="lightning-stat">
            <span className="lightning-count">{current.lightningStrikeCount}</span>
            <span className="lightning-label">strikes today</span>
          </div>
          <div className="lightning-stat">
            <span className={`lightning-distance ${hasStrikes ? '' : 'lightning-distance-none'}`}>
              {hasStrikes && current.lightningStrikeAvgDistance > 0
                ? `${current.lightningStrikeAvgDistance.toFixed(1)} km`
                : '—'}
            </span>
            <span className="lightning-label">avg distance</span>
          </div>
        </div>
        {!hasStrikes && (
          <div className="lightning-clear">
            <span className="lightning-clear-text">No lightning detected</span>
            <span className="lightning-range-text">Range: up to 40 km</span>
          </div>
        )}
      </div>

      {hasStrikes && (
        <div className="lightning-distance-rings">
          <div className="ring ring-10">10km</div>
          <div className="ring ring-20">20km</div>
          <div className="ring ring-40">40km</div>
          {current.lightningStrikeAvgDistance > 0 && (
            <div
              className="lightning-indicator"
              style={{
                top: `${Math.min(90, (current.lightningStrikeAvgDistance / 40) * 90)}%`,
              }}
            />
          )}
        </div>
      )}
    </GlassCard>
  );
}
