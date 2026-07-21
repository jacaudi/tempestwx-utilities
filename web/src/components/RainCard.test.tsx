import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/react';
import { RainCard } from './RainCard';
import { PrecipitationType, PressureTrend, type CurrentObservation } from '../types/weather';

const baseCurrent: CurrentObservation = {
  timestamp: 1700000000,
  windLull: 1,
  windAvg: 2,
  windGust: 3,
  windDirection: 180,
  windSampleInterval: 3,
  stationPressure: 1013,
  airTemperature: 20,
  relativeHumidity: 55,
  illuminance: 1000,
  uvIndex: 2,
  solarRadiation: 100,
  rainAccumulated: 0.5,
  precipitationType: PrecipitationType.Rain,
  lightningStrikeAvgDistance: 0,
  lightningStrikeCount: 0,
  battery: 2.6,
  reportInterval: 1,
  localDayRainAccumulation: 1.2,
  feelsLike: 20,
  dewPoint: 12,
  wetBulbTemperature: 15,
  heatIndex: 20,
  windChill: 20,
  pressureTrend: PressureTrend.Steady,
};

describe('RainCard raindrop stability (P2.13)', () => {
  it('keeps raindrop positions stable across a re-render with new (still-raining) props', () => {
    const { container, rerender } = render(<RainCard current={baseCurrent} unit="in" />);

    const before = Array.from(container.querySelectorAll<HTMLElement>('.raindrop')).map(
      (el) => el.style.left
    );
    expect(before).toHaveLength(12);

    rerender(
      <RainCard
        current={{ ...baseCurrent, localDayRainAccumulation: 2.4 }}
        unit="in"
      />
    );

    const after = Array.from(container.querySelectorAll<HTMLElement>('.raindrop')).map(
      (el) => el.style.left
    );

    expect(after).toEqual(before);
  });
});
