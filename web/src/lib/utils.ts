import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/**
 * 统一数值展示：先按量级降精度并给出单位后缀，再按该语义允许的小数位格式化。
 *
 * 改前所有语义一律 `toFixed(2)`，导致请求数、Token 数这类整数被渲染成
 * `42.00` / `1.23 K`。这里把「是否保留小数」交给调用方按语义决定：
 * - 计数：整数，不保留小数（1,234 → 1.23K 仍保留两位以表达量级）
 * - 金额：低于 1 元保留两位，避免出现 0.00 丢信息
 * - 耗时：整毫秒不保留小数，秒及以上保留两位
 */
function formatScaled(
  num: number | undefined,
  thresholds: number[],
  units: string[],
  decimals: (scaled: number, unit: string) => number,
): { value: string, unit: string } {
  if (num === undefined || !Number.isFinite(num)) {
    return { value: '0', unit: units[0] }
  }

  for (let i = 0; i < thresholds.length; i++) {
    if (num >= thresholds[i]) {
      const scaled = num / thresholds[i]
      return { value: scaled.toFixed(decimals(scaled, units[i + 1])), unit: units[i + 1] }
    }
  }

  const unit = units[units.length - 1]
  return { value: num.toFixed(decimals(num, unit)), unit }
}

/** 计数：整数值不带小数；换算到 K/M/B 后保留两位表达量级。 */
export function formatCount(num: number | undefined): { raw: number, formatted: { value: string, unit: string } } {
  const scaledUnits = new Set(['K', 'M', 'B'])
  return {
    raw: num ?? 0,
    formatted: formatScaled(num, [1000000000, 1000000, 1000], ['', 'B', 'M', 'K', ''], (_scaled, unit) =>
      scaledUnits.has(unit) ? 2 : 0,
    ),
  }
}

/** 金额：整元也保留两位，低于 1 元同样两位，避免 0.00。 */
export function formatMoney(num: number | undefined): { raw: number, formatted: { value: string, unit: string } } {
  return {
    raw: num ?? 0,
    formatted: formatScaled(num, [1000000000, 1000000, 1000], ['$', 'B$', 'M$', 'K$', '$'], () => 2),
  }
}

/** 耗时：整毫秒直接显示整数，秒/分钟/小时保留两位。 */
export function formatTime(ms: number | undefined): { raw: number, formatted: { value: string, unit: string } } {
  return {
    raw: ms ?? 0,
    formatted: formatScaled(ms, [86400000, 3600000, 60000, 1000], ['', 'd', 'h', 'm', 's', 'ms'], (_scaled, unit) =>
      unit === 'ms' ? 0 : 2,
    ),
  }
}
