/**
 * Tempest WeatherFlow API client
 *
 * REST API base: https://swd.weatherflow.com/swd/rest
 * WebSocket: wss://ws.weatherflow.com/swd/data
 *
 * All methods currently return stub data.
 * Replace with real fetch calls when connecting to the live API.
 */

import type {
  CurrentObservation,
  StationMeta,
  ForecastDay,
  HourlyForecast,
  StationStatus,
  StationAlmanac,
} from '../types/weather';
import {
  stubCurrentObservation,
  stubStationMeta,
  stubForecast,
  stubHourlyForecast,
  stubStationStatus,
  stubStationAlmanac,
} from './stubData';

const API_BASE = 'https://swd.weatherflow.com/swd/rest';

/**
 * In production, the token is obtained via:
 * 1. Personal Access Token (Settings → Data Authorizations → Create Token at tempestwx.com)
 * 2. OAuth flow for third-party apps
 */
let _apiToken: string | null = null;

export function setApiToken(token: string) {
  _apiToken = token;
}

export function getApiToken(): string | null {
  return _apiToken;
}

// ---------------------------------------------------------------------------
// Station metadata
// GET /stations?token={token}
// ---------------------------------------------------------------------------
export async function fetchStationMeta(
  _stationId?: number
): Promise<StationMeta> {
  // TODO: Replace with real API call
  // const res = await fetch(`${API_BASE}/stations?token=${_apiToken}`);
  // const data = await res.json();
  // return parseStationMeta(data);
  void _stationId;
  void API_BASE;
  return Promise.resolve({ ...stubStationMeta });
}

// ---------------------------------------------------------------------------
// Current observations
// GET /observations/stn/{station_id}?token={token}
// ---------------------------------------------------------------------------
export async function fetchCurrentObservation(
  _stationId?: number
): Promise<CurrentObservation> {
  // TODO: Replace with real API call
  // const res = await fetch(
  //   `${API_BASE}/observations/stn/${stationId}?token=${_apiToken}`
  // );
  // const data = await res.json();
  // return parseObservation(data.obs[0]);
  void _stationId;
  return Promise.resolve({
    ...stubCurrentObservation,
    timestamp: Date.now() / 1000,
  });
}

// ---------------------------------------------------------------------------
// Forecast
// GET /better_forecast?station_id={station_id}&token={token}
// ---------------------------------------------------------------------------
export async function fetchForecast(
  _stationId?: number
): Promise<ForecastDay[]> {
  // TODO: Replace with real API call
  // const res = await fetch(
  //   `${API_BASE}/better_forecast?station_id=${stationId}&token=${_apiToken}`
  // );
  // const data = await res.json();
  // return parseForecast(data.forecast.daily);
  void _stationId;
  return Promise.resolve([...stubForecast]);
}

// ---------------------------------------------------------------------------
// Hourly forecast
// GET /better_forecast?station_id={station_id}&token={token}
// (parsed from the hourly portion of the same response)
// ---------------------------------------------------------------------------
export async function fetchHourlyForecast(
  _stationId?: number
): Promise<HourlyForecast[]> {
  void _stationId;
  return Promise.resolve([...stubHourlyForecast]);
}

// ---------------------------------------------------------------------------
// Station status / health
// GET /observations/device/{device_id}?token={token}
// ---------------------------------------------------------------------------
export async function fetchStationStatus(
  _deviceId?: number
): Promise<StationStatus> {
  void _deviceId;
  return Promise.resolve({
    ...stubStationStatus,
    lastReport: Date.now() / 1000 - 45,
  });
}

// ---------------------------------------------------------------------------
// Station almanac (historical highs/lows)
// GET /observations/stn/{station_id}?token={token}&bucket=day|week|month|year
// ---------------------------------------------------------------------------
export async function fetchStationAlmanac(
  _stationId?: number
): Promise<StationAlmanac> {
  void _stationId;
  return Promise.resolve({ ...stubStationAlmanac });
}

// ---------------------------------------------------------------------------
// WebSocket connection for real-time data
// wss://ws.weatherflow.com/swd/data?token={token}
// ---------------------------------------------------------------------------
export function connectWebSocket(
  _deviceId: number,
  _onObservation: (obs: CurrentObservation) => void
): { close: () => void } {
  // TODO: Implement real WebSocket connection
  // const ws = new WebSocket(`wss://ws.weatherflow.com/swd/data?token=${_apiToken}`);
  // ws.onopen = () => {
  //   ws.send(JSON.stringify({
  //     type: 'listen_start',
  //     device_id: deviceId,
  //     id: 'tempest-display',
  //   }));
  // };
  // ws.onmessage = (event) => {
  //   const msg = JSON.parse(event.data);
  //   if (msg.type === 'obs_st') {
  //     onObservation(parseObservation(msg.obs[0]));
  //   }
  // };
  // return { close: () => ws.close() };

  // Stub: simulate real-time updates every 3 seconds (matching Tempest WebSocket cadence)
  let windDir = stubCurrentObservation.windDirection;
  let windAvg = stubCurrentObservation.windAvg;
  let windGust = stubCurrentObservation.windGust;
  let windLull = stubCurrentObservation.windLull;

  const interval = setInterval(() => {
    const jitter = (val: number, step: number, min = 0, max = Infinity) =>
      Math.min(max, Math.max(min, val + (Math.random() - 0.5) * step));

    // Wind direction drifts slowly, ±5° per tick
    windDir = (windDir + (Math.random() - 0.5) * 10 + 360) % 360;
    windAvg = jitter(windAvg, 0.4, 0, 20);
    windGust = jitter(windGust, 0.6, windAvg, 30);
    windLull = jitter(windLull, 0.3, 0, windAvg);

    _onObservation({
      ...stubCurrentObservation,
      timestamp: Date.now() / 1000,
      windAvg,
      windGust,
      windLull,
      windDirection: Math.round(windDir),
    });
  }, 3000);

  return { close: () => clearInterval(interval) };
}

