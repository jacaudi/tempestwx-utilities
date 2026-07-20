/**
 * Consistent SVG weather icons — replaces platform-dependent emoji.
 * All icons use a 48×48 viewBox and accept a `size` prop for scaling.
 */

interface WeatherIconProps {
  icon: string;
  size?: number;
}

const SUN    = '#fbbf24';
const CLOUD  = 'rgba(255,255,255,0.95)';
const CLOUD_NIGHT = 'rgba(190,210,255,0.88)';
const RAIN   = '#60a5fa';
const SNOW   = '#bfdbfe';
const BOLT   = '#fbbf24';
const MOON   = '#ddeeff';
const WIND_C = '#94a3b8';

/* ---- sub-shapes ---- */

function Sun({ cx, cy, r, rayLen = 5 }: { cx: number; cy: number; r: number; rayLen?: number }) {
  const rays = Array.from({ length: 8 }, (_, i) => {
    const a = (i * 45 - 90) * (Math.PI / 180);
    const gap = 3;
    const x1 = cx + Math.cos(a) * (r + gap);
    const y1 = cy + Math.sin(a) * (r + gap);
    const x2 = cx + Math.cos(a) * (r + gap + rayLen);
    const y2 = cy + Math.sin(a) * (r + gap + rayLen);
    return <line key={i} x1={x1} y1={y1} x2={x2} y2={y2} stroke={SUN} strokeWidth="2.2" strokeLinecap="round" />;
  });
  return (
    <>
      {rays}
      <circle cx={cx} cy={cy} r={r} fill={SUN} />
    </>
  );
}

function Cloud({ cx, cy, s = 1, color = CLOUD }: { cx: number; cy: number; s?: number; color?: string }) {
  return (
    <>
      <circle cx={cx - 7 * s} cy={cy + 2 * s} r={6.5 * s} fill={color} />
      <circle cx={cx}          cy={cy - 4 * s} r={8.5 * s} fill={color} />
      <circle cx={cx + 8 * s} cy={cy + 2 * s} r={6 * s}   fill={color} />
      <rect
        x={cx - 13.5 * s}
        y={cy + 2 * s}
        width={27 * s}
        height={7 * s}
        fill={color}
      />
    </>
  );
}

function Moon({ cx, cy, r }: { cx: number; cy: number; r: number }) {
  /* crescent via evenodd: outer circle minus offset inner circle */
  const ir  = r * 0.78;
  const off = r * 0.52;
  const oc = (sign: 1 | -1) =>
    `M ${cx} ${cy - r} A ${r} ${r} 0 1 ${sign === 1 ? 0 : 1} ${cx} ${cy + r} A ${r} ${r} 0 1 ${sign === 1 ? 0 : 1} ${cx} ${cy - r} Z`;
  const ic = (sign: 1 | -1) =>
    `M ${cx + off} ${cy - ir} A ${ir} ${ir} 0 1 ${sign === 1 ? 0 : 1} ${cx + off} ${cy + ir} A ${ir} ${ir} 0 1 ${sign === 1 ? 0 : 1} ${cx + off} ${cy - ir} Z`;
  return (
    <path fillRule="evenodd" fill={MOON} d={`${oc(1)} ${ic(1)}`} />
  );
}

function RainDrops({ cx, top }: { cx: number; top: number }) {
  return (
    <>
      {[-9, 0, 9, 18].map((dx, i) => (
        <line
          key={i}
          x1={cx + dx}     y1={top}
          x2={cx + dx - 3} y2={top + 9}
          stroke={RAIN} strokeWidth="2.2" strokeLinecap="round"
        />
      ))}
    </>
  );
}

function SnowFlakes({ cx, top }: { cx: number; top: number }) {
  return (
    <>
      {[-10, 0, 10].map((dx, i) => {
        const x = cx + dx;
        const y = top + 5;
        return (
          <g key={i}>
            <line x1={x} y1={y - 5} x2={x} y2={y + 5} stroke={SNOW} strokeWidth="2" strokeLinecap="round" />
            <line x1={x - 4} y1={y - 3} x2={x + 4} y2={y + 3} stroke={SNOW} strokeWidth="2" strokeLinecap="round" />
            <line x1={x + 4} y1={y - 3} x2={x - 4} y2={y + 3} stroke={SNOW} strokeWidth="2" strokeLinecap="round" />
          </g>
        );
      })}
    </>
  );
}

/* ---- main component ---- */

export function WeatherIcon({ icon, size = 48 }: WeatherIconProps) {
  let content: React.ReactNode;

  switch (icon) {
    case 'clear-day':
      content = <Sun cx={24} cy={24} r={10} rayLen={5} />;
      break;

    case 'clear-night':
      content = <Moon cx={24} cy={24} r={12} />;
      break;

    case 'partly-cloudy-day':
      content = (
        <>
          <Sun cx={31} cy={16} r={8} rayLen={4} />
          <Cloud cx={22} cy={32} s={1.1} />
        </>
      );
      break;

    case 'partly-cloudy-night':
      content = (
        <>
          <Moon cx={32} cy={15} r={9} />
          <Cloud cx={21} cy={32} s={1.1} color={CLOUD_NIGHT} />
        </>
      );
      break;

    case 'cloudy':
      content = <Cloud cx={24} cy={26} s={1.35} />;
      break;

    case 'rainy':
      content = (
        <>
          <Cloud cx={24} cy={21} s={1.2} />
          <RainDrops cx={24} top={35} />
        </>
      );
      break;

    case 'thunderstorm':
      content = (
        <>
          <Cloud cx={24} cy={19} s={1.2} />
          {/* Lightning bolt */}
          <path d="M27 30 L20 41 L25 41 L22 48 L32 36 L27 36 Z" fill={BOLT} />
        </>
      );
      break;

    case 'snow':
      content = (
        <>
          <Cloud cx={24} cy={21} s={1.2} />
          <SnowFlakes cx={24} top={34} />
        </>
      );
      break;

    case 'windy':
      content = (
        <>
          <Cloud cx={24} cy={17} s={1.0} />
          <path d="M6 30 Q14 26 22 30 Q30 34 38 30 Q42 28 44 30" stroke={WIND_C} strokeWidth="2.5" strokeLinecap="round" fill="none" />
          <path d="M6 38 Q13 34 20 38 Q27 42 34 38" stroke={WIND_C} strokeWidth="2.5" strokeLinecap="round" fill="none" />
        </>
      );
      break;

    default:
      content = (
        <>
          <Sun cx={31} cy={16} r={8} rayLen={4} />
          <Cloud cx={22} cy={32} s={1.1} />
        </>
      );
  }

  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 48 48"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
    >
      {content}
    </svg>
  );
}
