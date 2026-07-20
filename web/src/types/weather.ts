/** Tempest Weather Station data types based on the WeatherFlow API */

export interface StationMeta {
  station_id: number;
  name: string;
  latitude: number;
  longitude: number;
  elevation: number;
  timezone: string;
  firmware_revision: string;
  serial_number: string;
  device_id: number;
}

export interface CurrentObservation {
  timestamp: number;
  windLull: number;        // m/s
  windAvg: number;         // m/s
  windGust: number;        // m/s
  windDirection: number;   // degrees
  windSampleInterval: number; // seconds
  stationPressure: number; // MB
  airTemperature: number;  // °C
  relativeHumidity: number; // %
  illuminance: number;     // lux
  uvIndex: number;
  solarRadiation: number;  // W/m²
  rainAccumulated: number; // mm
  precipitationType: PrecipitationType;
  lightningStrikeAvgDistance: number; // km
  lightningStrikeCount: number;
  battery: number;         // Volts
  reportInterval: number;  // minutes
  localDayRainAccumulation: number; // mm
  feelsLike: number;       // °C (calculated)
  dewPoint: number;        // °C (calculated)
  wetBulbTemperature: number; // °C (calculated)
  heatIndex: number;       // °C (calculated)
  windChill: number;       // °C (calculated)
  pressureTrend: PressureTrend;
}

export const PrecipitationType = {
  None: 0,
  Rain: 1,
  Hail: 2,
  RainAndHail: 3,
} as const;
export type PrecipitationType = typeof PrecipitationType[keyof typeof PrecipitationType];

export const PressureTrend = {
  Falling: 'falling',
  Steady: 'steady',
  Rising: 'rising',
} as const;
export type PressureTrend = typeof PressureTrend[keyof typeof PressureTrend];

export interface ForecastDay {
  dayNum: number;
  monthNum: number;
  conditions: string;
  icon: string;
  airTempHigh: number;   // °C
  airTempLow: number;    // °C
  precipProbability: number; // %
  precipType: string;
  sunrise: number;       // epoch
  sunset: number;        // epoch
}

export interface HourlyForecast {
  timestamp: number;
  conditions: string;
  icon: string;
  airTemperature: number;
  feelsLike: number;
  relativeHumidity: number;
  windAvg: number;
  windDirection: number;
  windGust: number;
  precipProbability: number;
  uvIndex: number;
}

export interface StationStatus {
  isOnline: boolean;
  lastReport: number;
  batteryLevel: number;
  signalStrength: number; // 0-4
  firmwareVersion: string;
}

export type TemperatureUnit = 'C' | 'F';
export type WindUnit = 'ms' | 'mph' | 'kph' | 'kts';
export type PressureUnit = 'mb' | 'inHg' | 'hPa';
export type RainUnit = 'mm' | 'in';

export interface TempRecord {
  high: number;     // °C
  highDate: string; // human label, e.g. "Today", "Feb 15"
  low: number;      // °C
  lowDate: string;
}

export interface StationAlmanac {
  today: TempRecord;
  week: TempRecord;
  month: TempRecord;
  year: TempRecord;
  sunrise: number;        // epoch seconds
  sunset: number;         // epoch seconds
  moonPhase: number;      // 0–1 (0 = new, 0.5 = full)
  moonPhaseName: string;
  moonIllumination: number; // 0–1
}

export interface UserPreferences {
  temperatureUnit: TemperatureUnit;
  windUnit: WindUnit;
  pressureUnit: PressureUnit;
  rainUnit: RainUnit;
  theme: string;
}

export type ThemeName = 'liquid-glass' | 'midnight-aurora' | 'desert-sunset' | 'nord' | 'tokyo-night' | 'catppuccin-mocha' | 'the-grid';

