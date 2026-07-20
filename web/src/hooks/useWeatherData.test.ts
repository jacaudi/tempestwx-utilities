import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';
import { useWeatherData, POLL_INTERVAL_MS } from './useWeatherData';
import * as api from '../api/tempestApi';
import { PrecipitationType, PressureTrend } from '../types/weather';
import type {
  CurrentObservation,
  StationMeta,
  StationStatus,
  StationAlmanac,
} from '../types/weather';

vi.mock('../api/tempestApi', () => ({
  fetchCurrentObservation: vi.fn(),
  fetchStationMeta: vi.fn(),
  fetchForecast: vi.fn(),
  fetchHourlyForecast: vi.fn(),
  fetchStationStatus: vi.fn(),
  fetchStationAlmanac: vi.fn(),
}));

const baseObs: CurrentObservation = {
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

const baseStation: StationMeta = {
  station_id: 1,
  name: 'Test Station',
  latitude: 0,
  longitude: 0,
  elevation: 0,
  timezone: 'UTC',
  firmware_revision: 'v1',
  serial_number: 'ST-1',
  device_id: 1,
};

const baseStatus: StationStatus = {
  isOnline: true,
  lastReport: 1700000000,
  batteryLevel: 2.6,
  signalStrength: 0,
  firmwareVersion: '',
};

const baseAlmanac: StationAlmanac = {
  today: { high: 10, highDate: 'Today', low: 5, lowDate: 'Today' },
  week: { high: 10, highDate: 'Today', low: 5, lowDate: 'Today' },
  month: { high: 10, highDate: 'Today', low: 5, lowDate: 'Today' },
  year: { high: 10, highDate: 'Today', low: 5, lowDate: 'Today' },
  sunrise: 0,
  sunset: 0,
  moonPhase: 0.5,
  moonPhaseName: 'Full',
  moonIllumination: 1,
};

const mockedApi = vi.mocked(api);

beforeEach(() => {
  vi.resetAllMocks();
});

describe('useWeatherData', () => {
  it('retains prior data and flips isStale when a refetch of the core observation fails', async () => {
    mockedApi.fetchCurrentObservation
      .mockResolvedValueOnce(baseObs)
      .mockRejectedValueOnce(new Error('network down'));
    mockedApi.fetchStationMeta.mockResolvedValue(baseStation);
    mockedApi.fetchForecast.mockResolvedValue([]);
    mockedApi.fetchHourlyForecast.mockResolvedValue([]);
    mockedApi.fetchStationStatus.mockResolvedValue(baseStatus);
    mockedApi.fetchStationAlmanac.mockResolvedValue(baseAlmanac);

    const { result } = renderHook(() => useWeatherData());

    await waitFor(() => expect(result.current.current).toEqual(baseObs));
    expect(result.current.isStale).toBe(false);

    result.current.refresh();

    await waitFor(() => expect(result.current.isStale).toBe(true));
    expect(result.current.current).toEqual(baseObs);
    expect(mockedApi.fetchCurrentObservation).toHaveBeenCalledTimes(2);
  });

  it('keeps current populated when only the WeatherFlow-backed fetches fail (allSettled degradation)', async () => {
    mockedApi.fetchCurrentObservation.mockResolvedValue(baseObs);
    mockedApi.fetchStationMeta.mockRejectedValue(new Error('weatherflow down'));
    mockedApi.fetchForecast.mockRejectedValue(new Error('weatherflow down'));
    mockedApi.fetchHourlyForecast.mockRejectedValue(new Error('weatherflow down'));
    mockedApi.fetchStationStatus.mockResolvedValue(baseStatus);
    mockedApi.fetchStationAlmanac.mockRejectedValue(new Error('weatherflow down'));

    const { result } = renderHook(() => useWeatherData());

    await waitFor(() => expect(result.current.current).toEqual(baseObs));
    expect(result.current.isStale).toBe(false);
    expect(result.current.error).toBeNull();
    expect(result.current.station).toBeNull();
  });
});

describe('useWeatherData - isLoading with polling', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('clears isLoading when the poll interval aborts a still-in-flight initial load', async () => {
    // The initial fetchCurrentObservation call never resolves on its own --
    // it only settles (rejects, mirroring real fetch's abort behavior) once
    // its AbortSignal fires, simulating a hung/slow request. The second
    // call (issued by the poll tick) resolves immediately.
    let obsCallCount = 0;
    mockedApi.fetchCurrentObservation.mockImplementation(
      (_stationId?: number, signal?: AbortSignal) => {
        obsCallCount += 1;
        if (obsCallCount === 1) {
          return new Promise<CurrentObservation>((_resolve, reject) => {
            signal?.addEventListener('abort', () => {
              const err = new Error('Aborted');
              err.name = 'AbortError';
              reject(err);
            });
          });
        }
        return Promise.resolve(baseObs);
      }
    );
    mockedApi.fetchStationMeta.mockResolvedValue(baseStation);
    mockedApi.fetchForecast.mockResolvedValue([]);
    mockedApi.fetchHourlyForecast.mockResolvedValue([]);
    mockedApi.fetchStationStatus.mockResolvedValue(baseStatus);
    mockedApi.fetchStationAlmanac.mockResolvedValue(baseAlmanac);

    const { result } = renderHook(() => useWeatherData());

    expect(result.current.isLoading).toBe(true);

    // Advance past one poll tick: pollCurrent aborts the still-in-flight
    // initial loadData (its fetchCurrentObservation call rejects with
    // AbortError) and issues its own fetchCurrentObservation call, which
    // resolves.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
    });

    expect(result.current.current).toEqual(baseObs);
    expect(result.current.isLoading).toBe(false);
  });
});
