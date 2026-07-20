import type { CurrentObservation } from '../types/weather';
import { GlassCard } from './GlassCard';

interface SolarUVCardProps {
  current: CurrentObservation;
}

function uvLabel(uv: number): string {
  if (uv <= 2) return 'Low';
  if (uv <= 5) return 'Moderate';
  if (uv <= 7) return 'High';
  if (uv <= 10) return 'Very High';
  return 'Extreme';
}

function uvColorClass(uv: number): string {
  if (uv <= 2) return 'uv-low';
  if (uv <= 5) return 'uv-moderate';
  if (uv <= 7) return 'uv-high';
  if (uv <= 10) return 'uv-very-high';
  return 'uv-extreme';
}

function solarIntensity(radiation: number): string {
  if (radiation === 0) return 'None';
  if (radiation < 200) return 'Low';
  if (radiation < 500) return 'Moderate';
  if (radiation < 800) return 'High';
  return 'Very High';
}

export function SolarUVCard({ current }: SolarUVCardProps) {
  const uvPct = Math.min(100, (current.uvIndex / 11) * 100);

  return (
    <GlassCard className="solar-uv-card">
      <div className="card-header">
        <svg className="card-icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <circle cx="12" cy="12" r="5" />
          <line x1="12" y1="1" x2="12" y2="3" />
          <line x1="12" y1="21" x2="12" y2="23" />
          <line x1="4.22" y1="4.22" x2="5.64" y2="5.64" />
          <line x1="18.36" y1="18.36" x2="19.78" y2="19.78" />
          <line x1="1" y1="12" x2="3" y2="12" />
          <line x1="21" y1="12" x2="23" y2="12" />
          <line x1="4.22" y1="19.78" x2="5.64" y2="18.36" />
          <line x1="18.36" y1="5.64" x2="19.78" y2="4.22" />
        </svg>
        <span className="card-title">Solar &amp; UV</span>
      </div>

      <div className="solar-uv-grid">
        <div className="uv-section">
          <div className="uv-index-display">
            <span className={`uv-number ${uvColorClass(current.uvIndex)}`}>
              {current.uvIndex.toFixed(1)}
            </span>
            <span className="uv-label">{uvLabel(current.uvIndex)}</span>
          </div>
          <div className="uv-bar">
            <div className="uv-bar-track">
              <div className="uv-bar-fill" style={{ width: `${uvPct}%` }} />
              <div className="uv-bar-indicator" style={{ left: `${uvPct}%` }} />
            </div>
            <div className="uv-bar-labels">
              <span>0</span>
              <span>3</span>
              <span>6</span>
              <span>8</span>
              <span>11+</span>
            </div>
          </div>
        </div>

        <div className="solar-section">
          <div className="solar-stat">
            <span className="stat-label">Solar Radiation</span>
            <span className="stat-value">{Math.round(current.solarRadiation * 10) / 10} W/m²</span>
            <span className="stat-sublabel">{solarIntensity(current.solarRadiation)}</span>
          </div>
          <div className="solar-stat">
            <span className="stat-label">Illuminance</span>
            <span className="stat-value">{current.illuminance.toLocaleString()} lux</span>
          </div>
        </div>
      </div>
    </GlassCard>
  );
}
