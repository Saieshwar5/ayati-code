interface PerpetualPendulumMarkProps {
  className?: string;
}

export function PerpetualPendulumMark({ className = "" }: PerpetualPendulumMarkProps) {
  const classes = ["perpetual-mark", className].filter(Boolean).join(" ");

  return (
    <svg
      aria-hidden="true"
      className={classes}
      focusable="false"
      viewBox="0 0 28 28"
      xmlns="http://www.w3.org/2000/svg"
    >
      <circle className="perpetual-mark-pivot" cx="14" cy="4" r="2.25" />
      <circle className="perpetual-mark-pin" cx="14" cy="4" r="0.8" />
      <g className="perpetual-mark-pendulum">
        <path d="M14 6.5v16.25" />
        <path d="M14 13.25h3.1c3.15 0 5.15 1.8 5.15 4.65s-2 4.65-5.15 4.65H14" />
      </g>
    </svg>
  );
}
