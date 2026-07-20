import { describe, it, expect } from 'vitest';
import { formatCoord } from './formatCoord';

describe('formatCoord', () => {
  it('renders a northern, western station as N/W', () => {
    expect(formatCoord(40.7128, -74.006)).toBe('40.7128°N, 74.0060°W');
  });

  it('renders a southern, eastern station as S/E', () => {
    expect(formatCoord(-33.8688, 151.2093)).toBe('33.8688°S, 151.2093°E');
  });

  it('renders a southern, western station as S/W', () => {
    expect(formatCoord(-15.7801, -47.9292)).toBe('15.7801°S, 47.9292°W');
  });

  it('treats zero latitude/longitude as N/E', () => {
    expect(formatCoord(0, 0)).toBe('0.0000°N, 0.0000°E');
  });
});
