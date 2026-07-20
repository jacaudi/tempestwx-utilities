import { describe, it, expect, vi, afterEach } from 'vitest';
import { fetchCurrentObservation, fetchStationStatus } from './tempestApi';
import tempestApiSource from './tempestApi.ts?raw';
import { PrecipitationType, PressureTrend } from '../types/weather';
import type { CurrentObservation } from '../types/weather';

const mockObservation: CurrentObservation = {
  timestamp: 1700000000,
  windLull: 1.1,
  windAvg: 2.2,
  windGust: 3.3,
  windDirection: 180,
  windSampleInterval: 3,
  stationPressure: 1013.2,
  airTemperature: 15.5,
  relativeHumidity: 60,
  illuminance: 5000,
  uvIndex: 2,
  solarRadiation: 100,
  rainAccumulated: 0,
  precipitationType: PrecipitationType.None,
  lightningStrikeAvgDistance: 0,
  lightningStrikeCount: 0,
  battery: 2.6,
  reportInterval: 1,
  localDayRainAccumulation: 0,
  feelsLike: 15.5,
  dewPoint: 8,
  wetBulbTemperature: 11,
  heatIndex: 15.5,
  windChill: 15.5,
  pressureTrend: PressureTrend.Steady,
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('fetchCurrentObservation', () => {
  it('GETs the tokenless relative Contract C endpoint and returns a typed CurrentObservation', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve(mockObservation),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await fetchCurrentObservation();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('/api/observations/current');
    expect(init?.signal).toBeUndefined();
    expect(result).toEqual(mockObservation);
  });

  it('forwards an AbortSignal to fetch', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve(mockObservation),
    });
    vi.stubGlobal('fetch', fetchMock);
    const controller = new AbortController();

    await fetchCurrentObservation(undefined, controller.signal);

    expect(fetchMock.mock.calls[0][1]?.signal).toBe(controller.signal);
  });

  it('throws on a non-OK response instead of returning stub/garbage data', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 503 }));

    await expect(fetchCurrentObservation()).rejects.toThrow();
  });
});

describe('fetchStationStatus', () => {
  it('rejects instead of returning an offline default when the underlying fetch fails', async () => {
    // A transient server error must surface as a rejection so useWeatherData's
    // allSettled retains the prior good status, rather than fetchStationStatus
    // swallowing the error into a fake {isOnline:false, lastReport:0, ...}
    // default that would overwrite good prior state (M5).
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 503 }));

    await expect(fetchStationStatus()).rejects.toThrow();
  });

  it('derives an online StationStatus from a successful current-observation fetch', async () => {
    // mockObservation's fixed timestamp is years stale, which would derive
    // isOnline=false regardless of this fix -- use a fresh, "just reported"
    // observation instead so this test actually exercises the online branch.
    const freshObservation = { ...mockObservation, timestamp: Date.now() / 1000 };
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        json: () => Promise.resolve(freshObservation),
      })
    );

    const result = await fetchStationStatus();

    expect(result.isOnline).toBe(true);
    expect(result.lastReport).toBe(freshObservation.timestamp);
    expect(result.batteryLevel).toBe(mockObservation.battery);
  });
});

describe('module hygiene', () => {
  it('does not import the deleted stub data module', () => {
    expect(tempestApiSource).not.toMatch(/stubData/);
  });
});
