import { motion } from "motion/react";

const NODES = [
  { cx: 20, delay: 0 },
  { cx: 70, delay: 0.12 },
  { cx: 120, delay: 0.24 },
];

export function CanvasLoader() {
  return (
    <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/80 backdrop-blur-sm pointer-events-none">
      <div className="flex flex-col items-center gap-4">
        <svg
          width={140}
          height={50}
          viewBox="0 0 140 50"
          fill="none"
          aria-hidden
          className="overflow-visible"
        >
          <line
            x1={20}
            y1={25}
            x2={70}
            y2={25}
            className="stroke-border"
            strokeWidth={1.5}
            strokeDasharray="3 3"
          />
          <line
            x1={70}
            y1={25}
            x2={120}
            y2={25}
            className="stroke-border"
            strokeWidth={1.5}
            strokeDasharray="3 3"
          />

          {NODES.map(({ cx, delay }) => (
            <motion.rect
              key={cx}
              x={cx - 9}
              y={16}
              width={18}
              height={18}
              rx={4}
              className="fill-card stroke-border"
              strokeWidth={1.5}
              initial={{ opacity: 0, scale: 0.4, y: 4 }}
              animate={{ opacity: 1, scale: 1, y: 0 }}
              transition={{ delay, type: "spring", stiffness: 260, damping: 18 }}
              style={{ transformOrigin: "center", transformBox: "fill-box" }}
            />
          ))}

          <motion.circle
            r={3.5}
            cy={25}
            className="fill-primary"
            style={{ filter: "drop-shadow(0 0 6px var(--color-primary))" }}
            initial={{ cx: 20, opacity: 0 }}
            animate={{
              cx: [20, 70, 120, 70, 20],
              opacity: [0, 1, 1, 1, 0],
            }}
            transition={{
              delay: 0.4,
              duration: 2.4,
              repeat: Infinity,
              ease: "easeInOut",
              times: [0, 0.25, 0.5, 0.75, 1],
            }}
          />
        </svg>
        <motion.span
          className="text-sm text-muted-foreground"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.3 }}
        >
          Loading project canvas…
        </motion.span>
      </div>
    </div>
  );
}
