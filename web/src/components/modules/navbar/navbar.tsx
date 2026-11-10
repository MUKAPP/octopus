import { useRef, useState, type CSSProperties } from 'react';
import { motion, useReducedMotion } from 'motion/react';
import { flushSync } from 'react-dom';
import { cn } from '@/lib/utils';
import { useNavStore, type NavItem } from "@/components/modules/navbar"
import { ROUTES } from "@/route/config"
import { usePreload } from "@/route/use-preload"
import { DURATION, ENTRANCE_VARIANTS, REDUCED_MOTION_ENTRANCE_VARIANTS, REDUCED_MOTION_TRANSITION } from "@/lib/animations/fluid-transitions"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/animate-ui/components/animate/tooltip"
import { useTranslations } from "use-intl"
import { useIsMobile } from "@/hooks/use-mobile"

export function NavBar() {
    const { activeItem, setActiveItem } = useNavStore();
    const { preload } = usePreload();
    const t = useTranslations('navbar');
    const isMobile = useIsMobile();
    // 减少动态效果时取消逐项 stagger 与按压缩放，保留纯透明度过渡。
    const shouldReduceMotion = useReducedMotion() ?? false;
    const activeIndex = ROUTES.findIndex((route) => route.id === activeItem);
    const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
    const [isNavHovered, setIsNavHovered] = useState(false);
    const hoverIndicatorRef = useRef<HTMLSpanElement>(null);

    return (
        <div className="relative z-50 md:min-h-full">
            <motion.nav
                aria-label={t('ariaLabel')}
                className={cn(
                    "fixed bottom-[calc(1.5rem+var(--safe-area-bottom))] left-1/2 isolate -translate-x-1/2 flex items-center gap-1 p-3",
                    "md:sticky md:top-30 md:left-auto md:bottom-auto md:translate-x-0 md:flex-col md:gap-3",
                    "bg-sidebar text-sidebar-foreground border border-sidebar-border rounded-3xl",
                )}
                variants={shouldReduceMotion ? REDUCED_MOTION_ENTRANCE_VARIANTS.navbar : ENTRANCE_VARIANTS.navbar}
                initial="initial"
                animate="animate"
                onMouseLeave={() => setIsNavHovered(false)}
            >
                <span
                    aria-hidden="true"
                    className="pointer-events-none absolute left-3 top-3 z-10 size-10 rounded-2xl bg-sidebar-primary transition-transform duration-300 ease-out motion-reduce:transition-none [transform:translateX(var(--nav-offset-x))] md:size-12 md:[transform:translateY(var(--nav-offset-y))]"
                    style={{
                        '--nav-offset-x': `${activeIndex * 2.75}rem`,
                        '--nav-offset-y': `${activeIndex * 3.75}rem`,
                    } as CSSProperties}
                />
                <span
                    ref={hoverIndicatorRef}
                    aria-hidden="true"
                    className={cn(
                        'pointer-events-none absolute left-3 top-3 z-0 size-10 motion-reduce:transition-none [transform:translateX(var(--nav-offset-x))] md:size-12 md:[transform:translateY(var(--nav-offset-y))]',
                        isNavHovered ? 'transition-transform duration-300 ease-out' : 'transition-none',
                    )}
                    style={{
                        '--nav-offset-x': `${(hoveredIndex ?? activeIndex) * 2.75}rem`,
                        '--nav-offset-y': `${(hoveredIndex ?? activeIndex) * 3.75}rem`,
                    } as CSSProperties}
                >
                    <span
                        className="absolute inset-0 rounded-2xl bg-sidebar-accent transition-opacity duration-300 ease-linear motion-reduce:transition-none"
                        style={{ opacity: isNavHovered ? 1 : 0 }}
                    />
                </span>
                {ROUTES.map((route, index) => {
                    const isActive = activeItem === route.id;
                    const label = t(route.id);

                    return (
                        <Tooltip key={route.id} side={isMobile ? "top" : "right"} sideOffset={10}>
                            <TooltipTrigger asChild>
                                <motion.button
                                    type="button"
                                    aria-label={label}
                                    aria-current={isActive ? 'page' : undefined}
                                    onClick={() => {
                                        preload(route.id);
                                        setActiveItem(route.id as NavItem);
                                    }}
                                    onMouseEnter={() => {
                                        preload(route.id);
                                        if (!window.matchMedia('(hover: hover) and (pointer: fine)').matches) return;
                                        // 首次进入先在不可见状态下定位，避免背景从上一次位置移动过来。
                                        if (isNavHovered) {
                                            setHoveredIndex(index);
                                        } else {
                                            flushSync(() => setHoveredIndex(index));
                                            hoverIndicatorRef.current?.getBoundingClientRect();
                                            setIsNavHovered(true);
                                        }
                                    }}
                                    onFocus={() => preload(route.id)}
                                    onTouchStart={() => preload(route.id)}
                                    className={cn(
                                        "relative z-20 flex size-10 items-center justify-center rounded-2xl p-2 transition-[color,scale] duration-150 ease-linear motion-reduce:transition-none hover:z-30 [@media(hover:hover)_and_(pointer:fine)]:hover:scale-110 md:size-12 md:p-3",
                                        isActive ? "text-sidebar-primary-foreground" : "text-sidebar-foreground/75"
                                    )}
                                    initial={shouldReduceMotion ? { opacity: 0 } : { opacity: 0, scale: 0.8 }}
                                    animate={{
                                        opacity: 1,
                                        scale: 1,
                                        transition: shouldReduceMotion
                                            ? REDUCED_MOTION_TRANSITION
                                            : {
                                                delay: index * 0.05,
                                                duration: DURATION.page,
                                            }
                                    }}
                                    whileTap={shouldReduceMotion ? undefined : { scale: 0.95 }}
                                >
                                    <span className="relative z-10">
                                        <route.icon strokeWidth={2} aria-hidden="true" />
                                    </span>
                                </motion.button>
                            </TooltipTrigger>
                            <TooltipContent>{label}</TooltipContent>
                        </Tooltip>
                    );
                })}
            </motion.nav>
        </div>
    );
}
