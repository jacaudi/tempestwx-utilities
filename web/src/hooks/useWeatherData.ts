import { useState, useEffect, useCallback, useRef } from 'react';
import type {
  CurrentObservation,
  StationMeta,
  ForecastDay,
  HourlyForecast,
  StationStatus,
  StationAlmanac,
} from '../types/weather';
import {
  fetchCurrentObservation,
  fetchStationMeta,
  fetchForecast,
  fetchHourlyForecast,
  fetchStationStatus,
  fetchStationAlmanac,
} from '../api/tempestApi';

// There is no WebSocket backend (Contract C is plain JSON, see design §11),
// so live-ness comes from polling the core observation instead. 30s keeps
// the UI reasonably fresh against the station's own ~1-minute report
// cadence without hammering the read path; not derived from anything
// authoritative, just a reasonable middle ground.
export const POLL_INTERVAL_MS = 30_000;

export interface WeatherData {
  station: StationMeta | null;
  current: CurrentObservation | null;
  forecast: ForecastDay[];
  hourly: HourlyForecast[];
  status: StationStatus | null;
  almanac: StationAlmanac | null;
  isLoading: boolean;
  error: string | null;
  lastUpdated: Date | null;
  // True when the most recent attempt to refresh the core observation
  // failed and the data shown is therefore held over from an earlier,
  // successful fetch (§14 P1.6). False immediately after a successful
  // refresh.
  isStale: boolean;
  refresh: () => void;
}

// Applies a settled slice's result to its setter if it fulfilled, leaving
// prior state untouched otherwise -- the "retain on failure" rule every
// non-core slice (station/forecast/hourly/status/almanac) shares below.
function applySettled<T>(
  result: PromiseSettledResult<T>,
  setValue: (value: T) => void
): void {
  if (result.status === 'fulfilled') {
    setValue(result.value);
  }
}

function describeError(result: PromiseRejectedResult): string {
  return result.reason instanceof Error
    ? result.reason.message
    : 'Failed to load weather data';
}

export function useWeatherData(stationId?: number): WeatherData {
  const [station, setStation] = useState<StationMeta | null>(null);
  const [current, setCurrent] = useState<CurrentObservation | null>(null);
  const [forecast, setForecast] = useState<ForecastDay[]>([]);
  const [hourly, setHourly] = useState<HourlyForecast[]>([]);
  const [status, setStatus] = useState<StationStatus | null>(null);
  const [almanac, setAlmanac] = useState<StationAlmanac | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);
  const [isStale, setIsStale] = useState(false);

  // Tracks whichever request set (full load or a poll tick) is currently in
  // flight, so starting a new one cancels the old -- fixes the race where a
  // slow earlier request's response could land after, and clobber, a faster
  // later one (UI B-MEDIUM).
  const abortRef = useRef<AbortController | null>(null);

  const loadData = useCallback(async () => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    const { signal } = controller;

    setIsLoading(true);

    const [stationResult, obsResult, forecastResult, hourlyResult, statusResult, almanacResult] =
      await Promise.allSettled([
        fetchStationMeta(stationId, signal),
        fetchCurrentObservation(stationId, signal),
        fetchForecast(stationId, signal),
        fetchHourlyForecast(stationId, signal),
        fetchStationStatus(stationId, signal),
        fetchStationAlmanac(stationId, signal),
      ]);

    // Whether or not this run was superseded, it is over -- clear the
    // loading flag unconditionally so a poll tick that aborts a still-
    // in-flight initial load can never leave the spinner stuck forever (the
    // fetches would otherwise settle-as-rejected from the abort with
    // nothing left to clear it, since pollCurrent doesn't manage isLoading).
    setIsLoading(false);

    // This run was superseded by a newer loadData/poll call (which aborted
    // it) -- its DATA results are stale, so drop them (isLoading above still
    // needed clearing either way) instead of overwriting state the newer
    // call already wrote.
    if (signal.aborted) return;

    applySettled(stationResult, setStation);
    applySettled(forecastResult, setForecast);
    applySettled(hourlyResult, setHourly);
    applySettled(statusResult, setStatus);
    applySettled(almanacResult, setAlmanac);

    // isStale/error track only the core observation fetch -- the one
    // endpoint that actually works with no server TOKEN configured. The
    // WeatherFlow-backed slices (station/forecast/almanac) are documented
    // best-effort (design §11) and degrade silently: applySettled above
    // already left their prior value in place on failure.
    if (obsResult.status === 'fulfilled') {
      setCurrent(obsResult.value);
      setIsStale(false);
      setError(null);
      setLastUpdated(new Date());
    } else {
      setIsStale(true);
      setError(describeError(obsResult));
    }
  }, [stationId]);

  // Lightweight poll: refetches only the core observation, not the
  // WeatherFlow-backed slices -- there is no reason to hammer a best-effort
  // proxy on a timer. Shares abortRef with loadData so only one request set
  // is ever in flight.
  const pollCurrent = useCallback(async () => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    const { signal } = controller;

    try {
      const obs = await fetchCurrentObservation(stationId, signal);
      if (signal.aborted) return;
      setCurrent(obs);
      setIsStale(false);
      setError(null);
      setLastUpdated(new Date());
    } catch (err) {
      if (signal.aborted) return; // superseded/unmounted, not a real failure
      setIsStale(true);
      setError(err instanceof Error ? err.message : 'Failed to load weather data');
    }
  }, [stationId]);

  useEffect(() => {
    loadData();
    return () => abortRef.current?.abort();
  }, [loadData]);

  useEffect(() => {
    const id = setInterval(pollCurrent, POLL_INTERVAL_MS);
    return () => clearInterval(id);
  }, [pollCurrent]);

  return {
    station,
    current,
    forecast,
    hourly,
    status,
    almanac,
    isLoading,
    error,
    lastUpdated,
    isStale,
    refresh: loadData,
  };
}
