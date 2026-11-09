/**
 * 统一的文本复制实现。
 *
 * 为什么不直接用 `@uidotdev/usehooks` 的 `useCopyToClipboard`：它的实现会吞掉
 * `document.execCommand('copy')` 的布尔返回值，且 `handleCopy()` 不向外抛错，
 * 调用方永远拿不到失败信号，于是失败也会提示「复制成功」。
 *
 * 这里的契约是「只有明确成功才 resolve」：
 * 1. 优先使用异步 Clipboard API，并等待写入真正完成；
 * 2. 不可用或被拒绝时退回 execCommand 兜底，只有其返回 true 才算成功；
 * 3. 两条路径都失败时抛出原始错误，由调用方决定如何提示。
 */
export async function copyText(text: string): Promise<void> {
    try {
        if (!navigator.clipboard?.writeText) throw new Error('Clipboard API unavailable');
        await navigator.clipboard.writeText(text);
        return;
    } catch (cause) {
        if (copyWithExecCommand(text)) return;
        throw cause instanceof Error ? cause : new Error(String(cause));
    }
}

/**
 * execCommand 兜底：临时 textarea 承载文本并触发原生复制命令。
 * 无论结果如何都在 finally 中移除临时节点，不残留 DOM。
 */
export function copyWithExecCommand(text: string): boolean {
    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.setAttribute('readonly', '');
    // 移出视口，避免复制瞬间的可见闪烁与页面滚动跳动。
    textarea.style.position = 'fixed';
    textarea.style.top = '-9999px';
    textarea.style.opacity = '0';
    document.body.appendChild(textarea);
    try {
        textarea.select();
        return document.execCommand('copy');
    } catch {
        return false;
    } finally {
        textarea.remove();
    }
}
