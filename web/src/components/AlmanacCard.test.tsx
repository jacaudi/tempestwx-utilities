import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { AlmanacCard } from './AlmanacCard';
import type { StationAlmanac, TempRecord } from '../types/weather';

const RECORD: TempRecord = { high: 25, highDate: 'Today', low: 10, lowDate: 'Today' };

function makeAlmanac(overrides: Partial<StationAlmanac>): StationAlmanac {
  return {
    today: RECORD,
    week: RECORD,
    month: RECORD,
    year: RECORD,
    sunrise: 1_000_000,
    sunset: 1_000_000 + 14 * 3600 + 30 * 60, // 14h30m after sunrise
    moonPhase: 0.25,
    moonPhaseName: 'First Quarter',
    moonIllumination: 0.42,
    ...overrides,
  };
}

describe('AlmanacCard sunrise/sunset time formatting', () => {
  it('formats sunrise and sunset via the platform locale time formatter', () => {
    const almanac = makeAlmanac({});
    render(<AlmanacCard almanac={almanac} unit="F" />);

    const expectedSunrise = new Date(almanac.sunrise * 1000).toLocaleTimeString([], {
      hour: 'numeric',
      minute: '2-digit',
    });
    const expectedSunset = new Date(almanac.sunset * 1000).toLocaleTimeString([], {
      hour: 'numeric',
      minute: '2-digit',
    });

    expect(screen.getByText(expectedSunrise)).toBeInTheDocument();
    expect(screen.getByText(expectedSunset)).toBeInTheDocument();
  });
});

describe('AlmanacCard daylight duration', () => {
  it('renders hours and minutes between sunrise and sunset', () => {
    const almanac = makeAlmanac({});
    render(<AlmanacCard almanac={almanac} unit="F" />);
    expect(screen.getByText('14h 30m daylight')).toBeInTheDocument();
  });
});

describe('AlmanacCard moon phase', () => {
  it('renders the moon phase name and rounded illumination percentage', () => {
    const almanac = makeAlmanac({ moonPhaseName: 'Waxing Gibbous', moonIllumination: 0.678 });
    render(<AlmanacCard almanac={almanac} unit="F" />);
    expect(screen.getByText('Waxing Gibbous')).toBeInTheDocument();
    expect(screen.getByText('68% illuminated')).toBeInTheDocument();
  });
});
