/**
 * 格式化毫秒耗时：小于 1 秒显示 ms，否则显示秒
 */
export function formatDuration(ms: number): string {
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
}
