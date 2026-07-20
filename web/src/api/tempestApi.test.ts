import { describe, it, expect, vi, afterEach } from 'vitest';
import { fetchCurrentObservation } from './tempestApi';
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

describe('module hygiene', () => {
  it('does not import the deleted stub data module', () => {
    expect(tempestApiSource).not.toMatch(/stubData/);
  });
});
