import { AnimatePresence, motion } from "motion/react";
import { useEffect, useRef, useState } from "react";
import {
  siAngular,
  siAstro,
  siBun,
  siClickhouse,
  siDeno,
  siDjango,
  siExpress,
  siFastapi,
  siFlask,
  siGo,
  siInfluxdb,
  siLaravel,
  siMariadb,
  siMongodb,
  siMysql,
  siNeo4j,
  siNextdotjs,
  siNodedotjs,
  siNuxt,
  siPhp,
  siPostgresql,
  siPython,
  siQwik,
  siRabbitmq,
  siReact,
  siRedis,
  siRemix,
  siRubyonrails,
  siRust,
  type SimpleIcon,
  siSolid,
  siSpringboot,
  siSqlite,
  siSvelte,
  siVuedotjs,
} from "simple-icons";

const Y_CENTER = 120;
const NODE_SIZE = 88;
const HALF = NODE_SIZE / 2;

const ICON_POOL: SimpleIcon[] = [
  siNextdotjs,
  siLaravel,
  siNuxt,
  siSvelte,
  siRubyonrails,
  siAstro,
  siRemix,
  siDjango,
  siExpress,
  siVuedotjs,
  siReact,
  siAngular,
  siSolid,
  siQwik,
  siNodedotjs,
  siBun,
  siDeno,
  siPython,
  siGo,
  siRust,
  siPhp,
  siFastapi,
  siFlask,
  siSpringboot,
  siPostgresql,
  siMysql,
  siMariadb,
  siMongodb,
  siSqlite,
  siRedis,
  siRabbitmq,
  siClickhouse,
  siNeo4j,
  siInfluxdb,
];

function shuffle<T>(arr: T[]): T[] {
  const out = [...arr];
  for (let i = out.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [out[i], out[j]] = [out[j], out[i]];
  }
  return out;
}

function useRandomSlots(
  pool: SimpleIcon[],
  slotCount: number,
  intervalMs: number,
): SimpleIcon[] {
  const [slots, setSlots] = useState<SimpleIcon[]>(() =>
    shuffle(pool).slice(0, slotCount),
  );
  const queueRef = useRef<number[]>([]);
  const lastSlotRef = useRef<number>(-1);

  useEffect(() => {
    if (pool.length <= slotCount) return;
    const id = setInterval(() => {
      setSlots((prev) => {
        if (queueRef.current.length === 0) {
          const indices = Array.from({ length: slotCount }, (_, i) => i);
          const shuffled = shuffle(indices);
          if (shuffled[0] === lastSlotRef.current && shuffled.length > 1) {
            [shuffled[0], shuffled[1]] = [shuffled[1], shuffled[0]];
          }
          queueRef.current = shuffled;
        }
        const slotToUpdate = queueRef.current.shift() as number;
        lastSlotRef.current = slotToUpdate;

        const taken = new Set(
          prev.filter((_, i) => i !== slotToUpdate).map((icon) => icon.slug),
        );
        taken.add(prev[slotToUpdate].slug);
        const candidates = pool.filter((p) => !taken.has(p.slug));
        if (candidates.length === 0) return prev;
        const next = candidates[Math.floor(Math.random() * candidates.length)];

        const newSlots = [...prev];
        newSlots[slotToUpdate] = next;
        return newSlots;
      });
    }, intervalMs);
    return () => clearInterval(id);
  }, [pool, slotCount, intervalMs]);

  return slots;
}

function iconColor(icon: SimpleIcon): string {
  const r = parseInt(icon.hex.substring(0, 2), 16);
  const g = parseInt(icon.hex.substring(2, 4), 16);
  const b = parseInt(icon.hex.substring(4, 6), 16);
  const luminance = 0.299 * r + 0.587 * g + 0.114 * b;
  if (luminance < 80) return "var(--color-foreground)";
  return `#${icon.hex}`;
}

interface StackNodeProps {
  x: number;
  rotate: number;
  bobIndex: number;
  icon: SimpleIcon;
}

function StackNode({ x, rotate, bobIndex, icon }: StackNodeProps) {
  const cx = x + HALF;
  const cy = Y_CENTER;
  const color = iconColor(icon);

  return (
    <g
      className="animate-[welcome-node-bob_3.4s_ease-in-out_infinite]"
      style={{
        animationDelay: `${bobIndex * 0.5}s`,
        transformOrigin: "center",
      }}
    >
      <motion.g
        initial={{ rotate, scale: 1 }}
        whileHover={{ rotate: 0, scale: 1.1 }}
        transition={{ type: "spring", stiffness: 220, damping: 18 }}
        style={{
          transformOrigin: "center",
          transformBox: "fill-box",
          cursor: "pointer",
        }}
      >
        <AnimatePresence mode="wait" initial={false}>
          <motion.g
            key={icon.slug}
            initial={{ opacity: 0, scale: 0.4, rotate: 90 }}
            animate={{ opacity: 1, scale: 1, rotate: 0 }}
            exit={{ opacity: 0, scale: 0.4, rotate: -90 }}
            transition={{ duration: 0.5, ease: "easeOut" }}
            style={{
              transformOrigin: "center",
              transformBox: "fill-box",
            }}
          >
            <rect
              x={x}
              y={cy - HALF}
              width={NODE_SIZE}
              height={NODE_SIZE}
              rx={18}
              className="fill-white stroke-border dark:fill-card"
              strokeWidth={1.5}
              style={{
                filter:
                  "drop-shadow(0 12px 24px rgba(0,0,0,0.25)) drop-shadow(0 2px 4px rgba(0,0,0,0.15))",
              }}
            />
            <rect
              x={x}
              y={cy - HALF}
              width={NODE_SIZE}
              height={NODE_SIZE / 3}
              rx={18}
              className="fill-transparent dark:fill-primary/5"
            />

            <svg
              x={cx - 24}
              y={cy - 24}
              width={48}
              height={48}
              viewBox="0 0 24 24"
              role="img"
              aria-label={icon.title}
            >
              <path d={icon.path} fill={color} />
            </svg>

            <motion.circle
              cx={x + NODE_SIZE - 14}
              cy={cy - HALF + 14}
              r={4}
              className="fill-emerald-500"
              style={{ filter: "drop-shadow(0 0 6px #10b981)" }}
              animate={{ opacity: [0.5, 1, 0.5], scale: [0.85, 1, 0.85] }}
              transition={{
                duration: 1.6,
                delay: bobIndex * 0.5,
                repeat: Infinity,
                ease: "easeInOut",
              }}
            />
          </motion.g>
        </AnimatePresence>
      </motion.g>
    </g>
  );
}

export function WelcomeHero() {
  const slots = useRandomSlots(ICON_POOL, 3, 4000);

  return (
    <div className="relative mx-auto h-52 w-full text-primary">
      <div
        aria-hidden
        className="pointer-events-none absolute left-1/2 top-1/2 -z-10 hidden size-72 -translate-x-1/2 -translate-y-1/2 rounded-full bg-primary/8 blur-3xl dark:block"
      />
      <svg
        viewBox="0 0 480 240"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        className="h-full w-full overflow-visible"
        role="img"
        aria-label="Deploy any stack"
      >
        <StackNode x={20} rotate={-8} bobIndex={0} icon={slots[0]} />
        <StackNode x={196} rotate={6} bobIndex={1} icon={slots[1]} />
        <StackNode x={372} rotate={-7} bobIndex={2} icon={slots[2]} />
      </svg>
    </div>
  );
}
