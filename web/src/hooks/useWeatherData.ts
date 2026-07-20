import { useState, useEffect, useCallback } from 'react';
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
  connectWebSocket,
} from '../api/tempestApi';

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
  refresh: () => void;
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

  const loadData = useCallback(async () => {
    try {
      setIsLoading(true);
      setError(null);

      const [stationData, obsData, forecastData, hourlyData, statusData, almanacData] =
        await Promise.all([
          fetchStationMeta(stationId),
          fetchCurrentObservation(stationId),
          fetchForecast(stationId),
          fetchHourlyForecast(stationId),
          fetchStationStatus(stationId),
          fetchStationAlmanac(stationId),
        ]);

      setStation(stationData);
      setCurrent(obsData);
      setForecast(forecastData);
      setHourly(hourlyData);
      setStatus(statusData);
      setAlmanac(almanacData);
      setLastUpdated(new Date());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load weather data');
    } finally {
      setIsLoading(false);
    }
  }, [stationId]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  // Connect WebSocket for live updates
  useEffect(() => {
    if (!station) return;

    const ws = connectWebSocket(station.device_id, (obs) => {
      setCurrent(obs);
      setLastUpdated(new Date());
    });

    return () => ws.close();
  }, [station]);

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
    refresh: loadData,
  };
}
