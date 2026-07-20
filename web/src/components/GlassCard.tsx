import type { ReactNode, CSSProperties } from 'react';

interface GlassCardProps {
  children: ReactNode;
  className?: string;
  style?: CSSProperties;
  span?: 1 | 2 | 3;
  onClick?: () => void;
}

export function GlassCard({ children, className = '', style, span, onClick }: GlassCardProps) {
  const spanClass = span ? `card-span-${span}` : '';
  return (
    <div
      className={`glass-card ${spanClass} ${className}`}
      style={style}
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
    >
      {children}
    </div>
  );
}
