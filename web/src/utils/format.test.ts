import { describe, it, expect } from 'vitest';
import { formatX } from './format';

describe('formatX', () => {
  it('formats a defined numeric value using fmt', () => {
    expect(formatX(3.456, (n) => n.toFixed(1))).toBe('3.5');
  });

  it('renders an em-dash for undefined instead of throwing', () => {
    expect(formatX(undefined, (n) => n.toFixed(1))).toBe('—');
  });

  it('renders an em-dash for null instead of throwing', () => {
    expect(formatX(null, (n) => n.toFixed(1))).toBe('—');
  });

  it('renders an em-dash for NaN instead of throwing', () => {
    expect(formatX(NaN, (n) => n.toFixed(1))).toBe('—');
  });
});
