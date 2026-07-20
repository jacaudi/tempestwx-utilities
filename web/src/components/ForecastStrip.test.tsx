import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ForecastStrip } from './ForecastStrip';
import type { ForecastDay } from '../types/weather';

const DAY_NAMES = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

function makeDay(overrides: Partial<ForecastDay>): ForecastDay {
  return {
    dayNum: 1,
    monthNum: 1,
    conditions: 'Clear',
    icon: 'clear-day',
    airTempHigh: 20,
    airTempLow: 10,
    precipProbability: 0,
    precipType: 'none',
    sunrise: Math.floor(Date.now() / 1000),
    sunset: Math.floor(Date.now() / 1000) + 36000,
    ...overrides,
  };
}

afterEach(() => {
  vi.useRealTimers();
});

describe('ForecastStrip day names', () => {
  it("labels the first entry 'Today' regardless of its date", () => {
    render(
      <ForecastStrip
        unit="F"
        forecast={[makeDay({ dayNum: 20, monthNum: 12 })]}
      />
    );
    expect(screen.getByText('Today')).toBeInTheDocument();
  });

  it('derives the weekday from the forecast entry\'s own sunrise epoch, not the current year', () => {
    // "Now" is Dec 20, 2026 — a forecast entry for Jan 15, 2027 crosses the
    // Dec->Jan year boundary. The buggy implementation recomputes the date
    // using the CURRENT year (2026) instead of the forecast's actual year
    // (2027), which lands on a different weekday.
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 11, 20));

    const sunriseEpoch = Math.floor(new Date(2027, 0, 15, 13, 0, 0).getTime() / 1000);
    const expectedDayName = DAY_NAMES[new Date(sunriseEpoch * 1000).getDay()];

    render(
      <ForecastStrip
        unit="F"
        forecast={[
          makeDay({ dayNum: 20, monthNum: 12 }), // index 0 -> "Today", ignored
          makeDay({ dayNum: 15, monthNum: 1, sunrise: sunriseEpoch }),
        ]}
      />
    );

    expect(screen.getByText(expectedDayName)).toBeInTheDocument();
  });
});
