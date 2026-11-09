import { useEffect, useState, useRef } from 'react';
import { animate, useReducedMotion } from 'motion/react';
import { useLocale } from 'use-intl';

interface AnimatedNumberProps {
    value: string | number | undefined;
    duration?: number;
}

/**
 * 数字滚动展示。
 * - 小数位由传入值自身决定：整数不补零，带小数的保留其位数（最多两位）。
 * - 使用当前界面 locale 做千分位，避免英文界面固定 'en-US'。
 * - 系统开启「减少动态效果」时直接显示终值，不播放滚动。
 */
export function AnimatedNumber({ value, duration = 800 }: AnimatedNumberProps) {
    const locale = useLocale();
    const shouldReduceMotion = useReducedMotion() ?? false;
    const [displayValue, setDisplayValue] = useState(() => toNumber(value) ?? 0);
    const prevValueRef = useRef(displayValue);

    useEffect(() => {
        const numericValue = toNumber(value);
        if (numericValue === null) {
            // 无值时由渲染期回退到 0，这里只重置动画起点，不触发渲染。
            prevValueRef.current = 0;
            return;
        }

        if (shouldReduceMotion) {
            // 减少动态效果：不播放滚动，渲染期直接使用终值。
            prevValueRef.current = numericValue;
            return;
        }

        const controls = animate(prevValueRef.current, numericValue, {
            duration: duration / 1000, // motion/react uses seconds
            ease: 'easeOut',
            onUpdate: (latest) => {
                setDisplayValue(latest);
                prevValueRef.current = latest;
            }
        });

        return () => controls.stop();
    }, [value, duration, shouldReduceMotion]);

    if (value === undefined || value === null) {
        return <span aria-hidden="true">—</span>;
    }

    const finalValue = shouldReduceMotion ? (toNumber(value) ?? 0) : displayValue;
    const formattedValue = finalValue.toLocaleString(locale, {
        minimumFractionDigits: decimalPlacesOf(value),
        maximumFractionDigits: decimalPlacesOf(value),
    });

    return <span>{formattedValue}</span>;
}

/** 解析为数值；非数值与占位符返回 null。 */
function toNumber(value: string | number | undefined): number | null {
    if (value === undefined || value === null) return null;
    if (typeof value === 'number') return Number.isFinite(value) ? value : null;
    const trimmed = value.trim();
    if (trimmed === '' || trimmed === '—' || trimmed === '-') return null;
    const parsed = Number.parseFloat(trimmed.replace(/,/g, ''));
    return Number.isFinite(parsed) ? parsed : null;
}

/** 按传入值的小数位决定展示精度，整数不补零。 */
function decimalPlacesOf(value: string | number): number {
    if (typeof value !== 'string') return 0;
    const dot = value.indexOf('.');
    if (dot === -1) return 0;
    return Math.min(value.length - dot - 1, 2);
}
