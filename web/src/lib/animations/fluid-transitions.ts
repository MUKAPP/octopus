import type { Variants } from 'motion/react';

// 缓动函数
export const EASING = {
    easeOutCubic: [0.25, 0.46, 0.45, 0.94] as const,
    easeOutExpo: [0.16, 1, 0.3, 1] as const,
    easeInOutCubic: [0.65, 0, 0.35, 1] as const,
    easeOutQuart: [0.25, 1, 0.5, 1] as const,
} as const;

// Spring 配置
export const SPRING = {
    smooth: {
        type: "spring" as const,
        stiffness: 80,
        damping: 20,
        mass: 1.2,
    },
    gentle: {
        type: "spring" as const,
        stiffness: 70,
        damping: 18,
        mass: 1.5,
    },
    bouncy: {
        type: "spring" as const,
        stiffness: 100,
        damping: 15,
        mass: 1,
    },
    /**
     * 形变共享布局（layoutId morph）的统一参数。
     * 改前这组数值在 7 个文件里以字面量重复了 11 次，
     * 调用方应引用本项，避免各弹层手感漂移。
     */
    morph: {
        type: "spring" as const,
        stiffness: 400,
        damping: 30,
    },
    /**
     * 列表项进出场（如 API 密钥行）。比 morph 更硬，用于小尺寸元素的快速落位。
     */
    listItem: {
        type: "spring" as const,
        stiffness: 500,
        damping: 30,
    },
} as const;

/**
 * 统一的时长档位，与 globals.css 的 --duration-* 对应：
 * overlay 用于 tooltip/popover/select 等小浮层，panel 用于弹层外壳，page 用于页面过渡。
 */
export const DURATION = {
    overlay: 0.16,
    panel: 0.2,
    page: 0.3,
} as const;

/**
 * 固定入场动画：普通模式保留层级反馈，页面内容缩放不低于 0.95。
 */
export const ENTRANCE_VARIANTS = {
    navbar: {
        initial: {
            opacity: 0,
            scale: 0.2,
            filter: "blur(10px)",
        },
        animate: {
            opacity: 1,
            scale: 1,
            filter: "blur(0px)",
            transition: SPRING.smooth,
        },
    } as Variants,

    content: {
        initial: {
            scale: 0.95,
            opacity: 0,
        },
        animate: {
            scale: 1,
            opacity: 1,
            transition: {
                duration: DURATION.page,
                ease: EASING.easeOutExpo,
                delay: 0.1,
            },
        },
    } as Variants,

    header: {
        initial: {
            y: 100,
            opacity: 0,
            filter: "blur(10px)",
        },
        animate: {
            y: 0,
            opacity: 1,
            filter: "blur(0px)",
            transition: {
                duration: DURATION.page,
                ease: EASING.easeOutExpo,
                delay: 0.1,
            },
        },
    } as Variants,
};

export const REDUCED_MOTION_ENTRANCE_VARIANTS = {
    navbar: {
        initial: { opacity: 0 },
        animate: { opacity: 1, transition: { duration: 0.15, ease: EASING.easeOutCubic } },
    } as Variants,
    content: {
        initial: { opacity: 0 },
        animate: { opacity: 1, transition: { duration: 0.15, ease: EASING.easeOutCubic } },
    } as Variants,
    header: {
        initial: { opacity: 0 },
        animate: { opacity: 1, transition: { duration: 0.15, ease: EASING.easeOutCubic } },
    } as Variants,
};

export const ROUTE_TITLE_VARIANTS: Variants = {
    initial: (direction: number) => ({ y: 32 * direction, opacity: 0 }),
    animate: { y: 0, opacity: 1 },
    exit: (direction: number) => ({ y: -32 * direction, opacity: 0 }),
};

export const REDUCED_MOTION_ROUTE_TITLE_VARIANTS: Variants = {
    initial: { opacity: 0 },
    animate: { opacity: 1 },
    exit: { opacity: 0 },
};

export const REDUCED_MOTION_TRANSITION = {
    duration: 0.15,
    ease: EASING.easeOutCubic,
} as const;
