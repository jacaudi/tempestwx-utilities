import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { formatCoord } from './formatCoord';
import { Header } from './Header';

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

describe('Header stale indicator (M6a)', () => {
  const baseProps = {
    station: null,
    status: null,
    lastUpdated: new Date(2026, 0, 1, 12, 30),
    onSettingsClick: () => {},
  };

  it('renders a stale indicator when isStale is true', () => {
    render(<Header {...baseProps} isStale={true} />);
    expect(screen.getByText(/stale/i)).toBeInTheDocument();
  });

  it('does not render a stale indicator when isStale is false', () => {
    render(<Header {...baseProps} isStale={false} />);
    expect(screen.queryByText(/stale/i)).not.toBeInTheDocument();
  });
});
