import type { CurrentObservation, TemperatureUnit } from '../types/weather';
import { formatTemp } from '../hooks/useUnits';
import { GlassCard } from './GlassCard';
import { WeatherIcon } from './WeatherIcon';

interface TemperatureHeroProps {
  current: CurrentObservation;
  unit: TemperatureUnit;
  precipProbability?: number;
}

function getConditionLabel(obs: CurrentObservation): string {
  if (obs.lightningStrikeCount > 0) return 'Thunderstorm';
  if (obs.rainAccumulated > 0) return 'Rainy';
  if (obs.solarRadiation > 800) return 'Sunny';
  if (obs.solarRadiation > 400) return 'Partly Cloudy';
  if (obs.solarRadiation > 100) return 'Mostly Cloudy';
  if (obs.solarRadiation > 0) return 'Overcast';
  return 'Clear Night';
}

function getConditionIcon(obs: CurrentObservation): string {
  if (obs.lightningStrikeCount > 0) return 'thunderstorm';
  if (obs.rainAccumulated > 0) return 'rainy';
  if (obs.solarRadiation > 800) return 'clear-day';
  if (obs.solarRadiation > 400) return 'partly-cloudy-day';
  if (obs.solarRadiation > 100) return 'cloudy';
  if (obs.solarRadiation > 0) return 'cloudy';
  return 'clear-night';
}

export function TemperatureHero({ current, unit, precipProbability }: TemperatureHeroProps) {
  const condition = getConditionLabel(current);
  const icon = getConditionIcon(current);

  return (
    <GlassCard className="hero-card" span={2}>
      <div className="hero-content">
        <div className="hero-icon"><WeatherIcon icon={icon} size={72} /></div>
        <div className="hero-temp-block">
          <span className="hero-temp">{formatTemp(current.airTemperature, unit)}</span>
          <span className="hero-condition">{condition}</span>
          <span className="hero-feels-like">
            Feels like {formatTemp(current.feelsLike, unit)}
          </span>
        </div>
        <div className="hero-details">
          <div className="hero-detail-item">
            <span className="detail-label">Heat Index</span>
            <span className="detail-value">{formatTemp(current.heatIndex, unit)}</span>
          </div>
          <div className="hero-detail-item">
            <span className="detail-label">Wind Chill</span>
            <span className="detail-value">{formatTemp(current.windChill, unit)}</span>
          </div>
          <div className="hero-detail-item">
            <span className="detail-label">Rain Chance</span>
            <span className="detail-value">{Math.round(precipProbability ?? 0)}%</span>
          </div>
          <div className="hero-detail-item">
            <span className="detail-label">UV Index</span>
            <span className="detail-value">{current.uvIndex.toFixed(1)}</span>
          </div>
        </div>
      </div>
    </GlassCard>
  );
}
