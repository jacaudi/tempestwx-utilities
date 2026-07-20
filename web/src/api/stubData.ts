import type {
  CurrentObservation,
  StationMeta,
  ForecastDay,
  HourlyForecast,
  StationStatus,
  StationAlmanac,
} from '../types/weather';
import {
  PrecipitationType,
  PressureTrend,
} from '../types/weather';

export const stubStationMeta: StationMeta = {
  station_id: 12345,
  name: 'Shoreline Weather Station',
  latitude: 47.7559,
  longitude: -122.3417,
  elevation: 61,
  timezone: 'America/Los_Angeles',
  firmware_revision: 'v1.7.2',
  serial_number: 'ST-00012345',
  device_id: 67890,
};

export const stubCurrentObservation: CurrentObservation = {
  timestamp: Date.now() / 1000,
  windLull: 1.2,
  windAvg: 3.8,
  windGust: 7.4,
  windDirection: 212,       // SSW — prevailing direction for Seattle
  windSampleInterval: 6,
  stationPressure: 1018.4,
  airTemperature: 7.8,      // ~46°F, typical Feb Seattle
  relativeHumidity: 82,
  illuminance: 8200,        // overcast winter day
  uvIndex: 1.1,
  solarRadiation: 94,
  rainAccumulated: 1.8,     // light drizzle overnight
  precipitationType: PrecipitationType.Rain,
  lightningStrikeAvgDistance: 0,
  lightningStrikeCount: 0,
  battery: 2.61,
  reportInterval: 1,
  localDayRainAccumulation: 4.2,
  feelsLike: 4.9,
  dewPoint: 4.7,
  wetBulbTemperature: 6.1,
  heatIndex: 7.8,
  windChill: 4.9,
  pressureTrend: PressureTrend.Steady,
};

export const stubForecast: ForecastDay[] = [
  {
    dayNum: 18,
    monthNum: 2,
    conditions: 'Rainy',
    icon: 'rainy',
    airTempHigh: 9.2,
    airTempLow: 5.8,
    precipProbability: 90,
    precipType: 'rain',
    sunrise: Math.floor(Date.now() / 1000) - 3600 * 5,
    sunset: Math.floor(Date.now() / 1000) + 3600 * 5,
  },
  {
    dayNum: 19,
    monthNum: 2,
    conditions: 'Cloudy',
    icon: 'cloudy',
    airTempHigh: 10.1,
    airTempLow: 6.2,
    precipProbability: 65,
    precipType: 'rain',
    sunrise: Math.floor(Date.now() / 1000) + 3600 * 19,
    sunset: Math.floor(Date.now() / 1000) + 3600 * 29,
  },
  {
    dayNum: 20,
    monthNum: 2,
    conditions: 'Partly Cloudy',
    icon: 'partly-cloudy-day',
    airTempHigh: 11.4,
    airTempLow: 5.1,
    precipProbability: 30,
    precipType: 'rain',
    sunrise: Math.floor(Date.now() / 1000) + 3600 * 43,
    sunset: Math.floor(Date.now() / 1000) + 3600 * 53,
  },
  {
    dayNum: 21,
    monthNum: 2,
    conditions: 'Mostly Sunny',
    icon: 'clear-day',
    airTempHigh: 12.8,
    airTempLow: 4.4,
    precipProbability: 10,
    precipType: 'rain',
    sunrise: Math.floor(Date.now() / 1000) + 3600 * 67,
    sunset: Math.floor(Date.now() / 1000) + 3600 * 77,
  },
  {
    dayNum: 22,
    monthNum: 2,
    conditions: 'Rainy',
    icon: 'rainy',
    airTempHigh: 8.9,
    airTempLow: 5.6,
    precipProbability: 85,
    precipType: 'rain',
    sunrise: Math.floor(Date.now() / 1000) + 3600 * 91,
    sunset: Math.floor(Date.now() / 1000) + 3600 * 101,
  },
  {
    dayNum: 23,
    monthNum: 2,
    conditions: 'Cloudy',
    icon: 'cloudy',
    airTempHigh: 9.6,
    airTempLow: 5.3,
    precipProbability: 55,
    precipType: 'rain',
    sunrise: Math.floor(Date.now() / 1000) + 3600 * 115,
    sunset: Math.floor(Date.now() / 1000) + 3600 * 125,
  },
  {
    dayNum: 24,
    monthNum: 2,
    conditions: 'Rainy',
    icon: 'rainy',
    airTempHigh: 8.3,
    airTempLow: 4.9,
    precipProbability: 80,
    precipType: 'rain',
    sunrise: Math.floor(Date.now() / 1000) + 3600 * 139,
    sunset: Math.floor(Date.now() / 1000) + 3600 * 149,
  },
];

export const stubHourlyForecast: HourlyForecast[] = Array.from({ length: 24 }, (_, i) => ({
  timestamp: Math.floor(Date.now() / 1000) + i * 3600,
  conditions: i < 6 ? 'Clear' : i < 12 ? 'Partly Cloudy' : i < 18 ? 'Cloudy' : 'Clear',
  icon: i < 6 ? 'clear-night' : i < 12 ? 'partly-cloudy-day' : i < 18 ? 'cloudy' : 'clear-night',
  airTemperature: 18 + Math.sin((i / 24) * Math.PI * 2 - Math.PI / 2) * 6,
  feelsLike: 17.5 + Math.sin((i / 24) * Math.PI * 2 - Math.PI / 2) * 6.5,
  relativeHumidity: 55 + Math.cos((i / 24) * Math.PI * 2) * 20,
  windAvg: 1.5 + Math.random() * 3,
  windDirection: 180 + Math.floor(Math.random() * 90),
  windGust: 3 + Math.random() * 5,
  precipProbability: i >= 12 && i <= 16 ? 30 + Math.random() * 40 : Math.random() * 10,
  uvIndex: i >= 6 && i <= 18 ? Math.sin(((i - 6) / 12) * Math.PI) * 8 : 0,
}));

function todayAt(h: number, m: number): number {
  const d = new Date(); d.setHours(h, m, 0, 0); return d.getTime() / 1000;
}

export const stubStationAlmanac: StationAlmanac = {
  today:  { high: 9.4,  highDate: 'Feb 18', low: 5.1,  lowDate: 'Feb 18' },
  week:   { high: 12.8, highDate: 'Feb 21', low: 3.9,  lowDate: 'Feb 17' },
  month:  { high: 13.6, highDate: 'Feb 4',  low: 1.2,  lowDate: 'Feb 9'  },
  year:   { high: 17.2, highDate: 'Jan 13', low: -1.4, lowDate: 'Jan 18' },
  sunrise: todayAt(7, 9),   // Seattle Feb 18 sunrise
  sunset:  todayAt(17, 49), // Seattle Feb 18 sunset
  moonPhase: 0.70,
  moonPhaseName: 'Waning Gibbous',
  moonIllumination: 0.82,
};

export const stubStationStatus: StationStatus = {
  isOnline: true,
  lastReport: Date.now() / 1000 - 45,
  batteryLevel: 2.62,
  signalStrength: 3,
  firmwareVersion: 'v1.7.2',
};
