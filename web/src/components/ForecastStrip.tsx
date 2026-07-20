import type { ForecastDay, TemperatureUnit } from '../types/weather';
import { formatTemp } from '../hooks/useUnits';
import { GlassCard } from './GlassCard';
import { WeatherIcon } from './WeatherIcon';

interface ForecastStripProps {
  forecast: ForecastDay[];
  unit: TemperatureUnit;
}

const MONTH_NAMES = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
  'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
const DAY_NAMES = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];


function getDayName(dayNum: number, monthNum: number): string {
  const now = new Date();
  const date = new Date(now.getFullYear(), monthNum - 1, dayNum);
  return DAY_NAMES[date.getDay()];
}

export function ForecastStrip({ forecast, unit }: ForecastStripProps) {
  return (
    <GlassCard className="forecast-card" span={3}>
      <div className="card-header">
        <svg className="card-icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <rect x="3" y="4" width="18" height="18" rx="2" ry="2" />
          <line x1="16" y1="2" x2="16" y2="6" />
          <line x1="8" y1="2" x2="8" y2="6" />
          <line x1="3" y1="10" x2="21" y2="10" />
        </svg>
        <span className="card-title">7-Day Forecast</span>
      </div>
      <div className="forecast-strip">
        {forecast.map((day, i) => (
          <div key={i} className={`forecast-day ${i === 0 ? 'forecast-today' : ''}`}>
            <span className="forecast-day-name">
              {i === 0 ? 'Today' : getDayName(day.dayNum, day.monthNum)}
            </span>
            <span className="forecast-date">
              {MONTH_NAMES[day.monthNum - 1]} {day.dayNum}
            </span>
            <div className="forecast-icon"><WeatherIcon icon={day.icon} size={36} /></div>
            <span className="forecast-condition">{day.conditions}</span>
            <div className="forecast-temps">
              <span className="forecast-high">{formatTemp(day.airTempHigh, unit)}</span>
              <span className="forecast-low">{formatTemp(day.airTempLow, unit)}</span>
            </div>
            {day.precipProbability > 0 && (
              <span className="forecast-precip">
                <svg width="10" height="10" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M12 2.69l5.66 5.66a8 8 0 1 1-11.31 0z" />
                </svg>
                {day.precipProbability}%
              </span>
            )}
          </div>
        ))}
      </div>
    </GlassCard>
  );
}
