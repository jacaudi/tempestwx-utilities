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
    mockedApi.fetchStationStatus.mockResolvedValue(baseStatus);
    mockedApi.fetchStationAlmanac.mockRejectedValue(new Error('weatherflow down'));

    const { result } = renderHook(() => useWeatherData());

    await waitFor(() => expect(result.current.current).toEqual(baseObs));
    expect(result.current.isStale).toBe(false);
    expect(result.current.error).toBeNull();
    expect(result.current.station).toBeNull();
  });

  it('retains the prior station status on a subsequent status-fetch failure instead of overwriting it with an offline default (M5)', async () => {
    mockedApi.fetchCurrentObservation.mockResolvedValue(baseObs);
    mockedApi.fetchStationMeta.mockResolvedValue(baseStation);
    mockedApi.fetchForecast.mockResolvedValue([]);
    mockedApi.fetchStationStatus
      .mockResolvedValueOnce(baseStatus)
      .mockRejectedValueOnce(new Error('status endpoint down'));
    mockedApi.fetchStationAlmanac.mockResolvedValue(baseAlmanac);

    const { result } = renderHook(() => useWeatherData());

    await waitFor(() => expect(result.current.status).toEqual(baseStatus));

    result.current.refresh();

    await waitFor(() => expect(result.current.current).toEqual(baseObs));
    // The status fetch on this second run rejected -- the prior good status
    // must be retained (not overwritten with the offline default), and the
    // observation slice must remain populated.
    expect(result.current.status).toEqual(baseStatus);
    expect(mockedApi.fetchStationStatus).toHaveBeenCalledTimes(2);
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

  it('does not clear isLoading for a run that a newer refresh() call has already superseded', async () => {
    // First loadData's fetchCurrentObservation hangs until aborted (mirrors
    // real fetch abort behavior); the second (superseding) call also hangs
    // until manually resolved, so we can assert isLoading is still true
    // while it is in flight -- proving the aborted first run did not clear
    // the spinner out from under it.
    let obsCallCount = 0;
    let resolveSecondCall: ((obs: CurrentObservation) => void) | undefined;
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
        return new Promise<CurrentObservation>((resolve) => {
          resolveSecondCall = resolve;
        });
      }
    );
    mockedApi.fetchStationMeta.mockResolvedValue(baseStation);
    mockedApi.fetchForecast.mockResolvedValue([]);
    mockedApi.fetchStationStatus.mockResolvedValue(baseStatus);
    mockedApi.fetchStationAlmanac.mockResolvedValue(baseAlmanac);

    const { result } = renderHook(() => useWeatherData());

    expect(result.current.isLoading).toBe(true);

    // Trigger a second, superseding loadData run (aborts the first, in-flight
    // run) while the first run's fetchCurrentObservation is still pending.
    await act(async () => {
      result.current.refresh();
    });

    // The first run's allSettled has now resolved (its fetch rejected from
    // the abort), but the second run is still in flight -- isLoading must
    // still be true.
    expect(result.current.isLoading).toBe(true);

    // Let the second run's fetch resolve, completing it. Fake timers are
    // active in this describe block, so testing-library's `waitFor` (which
    // polls via setTimeout) would hang -- flush microtasks directly instead,
    // matching the pattern the sibling test above uses.
    await act(async () => {
      resolveSecondCall?.(baseObs);
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(result.current.isLoading).toBe(false);
    expect(result.current.current).toEqual(baseObs);
  });
});
