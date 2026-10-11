import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react';
import { AnimatePresence, motion } from 'motion/react';
import { AlertTriangle, Braces, Check, ChevronDown, Copy, Download, ExternalLink, FileText, Loader2, X } from 'lucide-react';
import JsonView from '@uiw/react-json-view';
import { githubDarkTheme } from '@uiw/react-json-view/githubDark';
import { githubLightTheme } from '@uiw/react-json-view/githubLight';
import { useLocale, useTranslations } from 'use-intl';
import type { LogContentOmission, LogContentPreview, LogResponseImage } from '@/api/endpoints/log';
import { fetchLogImage } from '@/api/endpoints/log';
import { useTheme } from '@/provider/theme';
import { MorphingDialogOverlayLayer, useMorphingDialog } from '@/components/ui/morphing-dialog';
import { toast } from '@/components/common/Toast';
import { cn } from '@/lib/utils';

/** 树视图默认展开到该层级，更深的节点折叠并可逐个展开。 */
const initialExpandLevel = 2;

/** 1024 进制的 IEC 标准符号：B/KiB/MiB/GiB，避免与十进制 kB 混淆。 */
const BYTE_UNITS = [
    { symbol: 'B', divisor: 1, maximumFractionDigits: 0 },
    { symbol: 'KiB', divisor: 1024, maximumFractionDigits: 1 },
    { symbol: 'MiB', divisor: 1024 ** 2, maximumFractionDigits: 1 },
    { symbol: 'GiB', divisor: 1024 ** 3, maximumFractionDigits: 1 },
] as const;

/** 按当前语言格式化字节数并附 IEC 符号；超出已知量级时停在 GiB 档继续换算。 */
function formatBytes(bytes: number, locale: string): string {
    if (!Number.isFinite(bytes) || bytes < 0) return '—';
    let chosen: (typeof BYTE_UNITS)[number] = BYTE_UNITS[0];
    for (const candidate of BYTE_UNITS) {
        if (bytes >= candidate.divisor) chosen = candidate;
    }
    const scaled = bytes / chosen.divisor;
    const maximumFractionDigits = Number.isInteger(scaled) ? 0 : chosen.maximumFractionDigits;
    return `${new Intl.NumberFormat(locale, { maximumFractionDigits }).format(scaled)} ${chosen.symbol}`;
}

function formatCount(value: number, locale: string): string {
    if (!Number.isFinite(value)) return '—';
    return new Intl.NumberFormat(locale).format(value);
}

/** 依 JSON Pointer 规则拼接键路径；根为 ""。 */
function pointerFromKeys(keys: readonly (string | number)[] | undefined): string {
    if (!keys || keys.length === 0) return '';
    return `/${keys.map((key) => String(key).replace(/~/g, '~0').replace(/\//g, '~1')).join('/')}`;
}

const OMISSION_KIND_KEYS = {
    string: 'kindString',
    array: 'kindArray',
    object: 'kindObject',
    binary: 'kindBinary',
} as const;

/**
 * 部分响应提示：默认只展示简短说明，大小、原因和省略字段通过详情展开。
 * 缺少保存元数据的旧记录返回 null，由正文面板沿用原有的截断徽标。
 */
function LogLossNotice({ preview, truncated }: { preview?: LogContentPreview; truncated?: boolean }) {
    const t = useTranslations('log.card');
    const locale = useLocale();
    if (preview === undefined) return null;

    const omissions = preview.omissions ?? [];
    const omittedDetails = preview.omitted_details ?? 0;
    const isLossy = truncated === true
        || preview.reason !== undefined
        || omissions.length > 0
        || omittedDetails > 0
        || preview.original_bytes !== preview.stored_bytes;
    if (!isLossy) return null;

    const reason = preview.reason === 'size_limit'
        ? t('reasonSizeLimit')
        : preview.reason === 'invalid_utf8'
            ? t('reasonInvalidUtf8')
            : undefined;

    return (
        <div className="flex flex-col gap-1.5 border-b border-border/70 bg-amber-500/5 px-3 py-2.5 text-xs md:px-4">
            <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                <AlertTriangle className="size-3.5 shrink-0 text-amber-600 dark:text-amber-400" aria-hidden="true" />
                <span className="font-medium text-amber-700 dark:text-amber-300">{t('lossyPreview')}</span>
            </div>
            <p className="min-w-0 break-words text-muted-foreground">{t('partialContentHint')}</p>
            <details className="min-w-0 text-muted-foreground">
                <summary className="cursor-pointer rounded-sm hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring">{t('previewDetails')}</summary>
                <p className="mt-1.5 min-w-0 break-words tabular-nums text-muted-foreground">
                    {t('originalSize')} {formatBytes(preview.original_bytes, locale)}
                    {' · '}
                    {t('storedSize')} {formatBytes(preview.stored_bytes, locale)}
                    {reason && <>{' · '}{reason}</>}
                </p>
                {(omissions.length > 0 || omittedDetails > 0) && (
                    <div className="mt-1.5 flex flex-col gap-1">
                        <span className="text-muted-foreground">{t('omittedFields')}</span>
                        {omissions.length > 0 && (
                            <ul className="scrollbar flex max-h-32 flex-col gap-1 overflow-y-auto overscroll-contain pr-1">
                                {omissions.map((omission) => (
                                    <li
                                        key={`${omission.path}-${omission.kind}`}
                                        className="flex min-w-0 flex-wrap items-baseline gap-x-1.5 gap-y-0.5"
                                    >
                                        <code className="min-w-0 break-all rounded bg-muted px-1 py-0.5 font-mono text-[11px] text-foreground">
                                            {omission.path === '' ? t('rootPath') : omission.path}
                                        </code>
                                        <span className="shrink-0 text-muted-foreground">{t(OMISSION_KIND_KEYS[omission.kind])}</span>
                                        <span className="min-w-0 break-words tabular-nums text-muted-foreground">
                                            {omission.original_items !== undefined || omission.stored_items !== undefined
                                                ? `${formatCount(omission.original_items ?? 0, locale)} → ${formatCount(omission.stored_items ?? 0, locale)} ${t('itemsUnit')}`
                                                : `${formatBytes(omission.original_bytes ?? 0, locale)} → ${formatBytes(omission.stored_bytes ?? 0, locale)}`}
                                        </span>
                                    </li>
                                ))}
                            </ul>
                        )}
                        {omittedDetails > 0 && (
                            <span className="text-muted-foreground">{t('omittedDetailsMore', { count: omittedDetails })}</span>
                        )}
                    </div>
                )}
            </details>
        </div>
    );
}

type BodyView = 'tree' | 'text';

/**
 * 无 Clipboard API 或被拒绝时的兜底：临时 textarea 承载文本并触发原生复制命令。
 * execCommand 返回 false 或抛错都判定为复制失败，只有明确成功才返回 true。
 * 无论结果如何都在 finally 中移除临时节点并恢复复制按钮焦点，不残留 DOM。
 */
function copyTextWithExecCommand(text: string, restoreFocus: () => void): boolean {
    const textarea = document.createElement('textarea');
    textarea.value = text;
    document.body.appendChild(textarea);
    try {
        textarea.select();
        return document.execCommand('copy');
    } catch {
        return false;
    } finally {
        textarea.remove();
        restoreFocus();
    }
}

const imageFileExtensions: Record<string, string> = {
    'image/png': 'png',
    'image/jpeg': 'jpg',
    'image/webp': 'webp',
    'image/gif': 'gif',
    'image/avif': 'avif',
    'image/svg+xml': 'svg',
};

/** 单张生成图片：独立放大与保存原图，关闭图片不会关闭日志详情。 */
function GeneratedImage({ src, number }: { src: string; number: number }) {
    const t = useTranslations('log.card');
    const [failedSrc, setFailedSrc] = useState<string | null>(null);
    const [viewerOpen, setViewerOpen] = useState(false);
    const [saving, setSaving] = useState(false);
    const viewerId = useId();
    const failed = failedSrc === src;
    const closeViewer = useCallback(() => setViewerOpen(false), []);

    const handleSave = async () => {
        setSaving(true);
        try {
            const blob = await fetchLogImage(src);
            const extension = imageFileExtensions[blob.type];
            const url = URL.createObjectURL(blob);
            const anchor = document.createElement('a');
            try {
                anchor.href = url;
                anchor.download = `octopus-image-${number}${extension ? `.${extension}` : ''}`;
                document.body.appendChild(anchor);
                anchor.click();
            } finally {
                anchor.remove();
                URL.revokeObjectURL(url);
            }
        } catch {
            toast.error(t('imageSaveFailed'), { description: t('imageSaveFailedHint') });
        } finally {
            setSaving(false);
        }
    };

    const imageActions = (
        <div className="ml-auto flex shrink-0 items-center gap-2">
            <button
                type="button"
                onClick={() => void handleSave()}
                disabled={saving || failed}
                aria-busy={saving}
                className="flex min-h-11 touch-manipulation select-none items-center gap-1.5 rounded-md px-3 text-xs hover:bg-muted/50 focus-visible:outline-2 focus-visible:outline-ring active:bg-muted/50 disabled:opacity-50"
            >
                {saving ? <Loader2 className="size-4 animate-spin" aria-hidden="true" /> : <Download className="size-4" aria-hidden="true" />}
                {t('saveImage')}
            </button>
            <a
                href={src}
                target="_blank"
                rel="noopener noreferrer"
                referrerPolicy="no-referrer"
                className="flex min-h-11 touch-manipulation select-none items-center gap-1.5 rounded-md px-3 text-xs hover:bg-muted/50 focus-visible:outline-2 focus-visible:outline-ring active:bg-muted/50"
            >
                <ExternalLink className="size-4" aria-hidden="true" />
                {t('openOriginalImage')}
            </a>
        </div>
    );

    return (
        <div className="flex min-w-0 flex-col gap-1">
            {failed ? (
                <p className="px-3 py-6 text-center text-xs text-destructive">{t('imageLoadFailed')}</p>
            ) : (
                <button
                    type="button"
                    aria-label={t('zoomImage', { number })}
                    onClick={(event) => { event.currentTarget.focus(); setViewerOpen(true); }}
                    className="block w-full cursor-zoom-in touch-manipulation rounded-md focus-visible:outline-2 focus-visible:outline-ring"
                >
                    <img
                        src={src}
                        alt={t('generatedImage', { number })}
                        loading="lazy"
                        decoding="async"
                        draggable={false}
                        referrerPolicy="no-referrer"
                        onError={() => setFailedSrc(src)}
                        onLoad={() => setFailedSrc(null)}
                        className="mx-auto max-h-[40dvh] w-full object-contain"
                    />
                </button>
            )}
            {imageActions}
            {viewerOpen && (
                <MorphingDialogOverlayLayer
                    layoutId={viewerId}
                    transition={{ duration: 0 }}
                    onClose={closeViewer}
                    panelClassName="h-full w-full rounded-none border-0 bg-background"
                >
                    <div role="dialog" aria-modal="true" aria-label={t('generatedImage', { number })} className="flex h-full min-h-0 flex-col">
                        <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-border px-3 py-2">
                            <button
                                type="button"
                                onClick={closeViewer}
                                aria-label={t('closeImage')}
                                className="flex size-11 shrink-0 touch-manipulation items-center justify-center rounded-md hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring active:bg-muted"
                            >
                                <X className="size-5" aria-hidden="true" />
                            </button>
                            <span className="min-w-0 flex-1 text-sm">{t('generatedImage', { number })}</span>
                            {imageActions}
                        </div>
                        <div
                            onClick={(event) => { if (event.target === event.currentTarget) closeViewer(); }}
                            className="flex min-h-0 flex-1 items-center justify-center overflow-auto overscroll-contain p-4"
                        >
                            {failed ? (
                                <p className="text-center text-sm text-destructive">{t('imageLoadFailed')}</p>
                            ) : (
                                <img
                                    src={src}
                                    alt={t('generatedImage', { number })}
                                    decoding="async"
                                    referrerPolicy="no-referrer"
                                    onError={() => setFailedSrc(src)}
                                    className="max-h-full max-w-full object-contain"
                                />
                            )}
                        </div>
                    </div>
                </MorphingDialogOverlayLayer>
            )}
        </div>
    );
}

/**
 * 生成图片预览区：独立折叠状态，默认展开；收起时保留已加载的图片节点。
 */
function LogImagePreview({ images }: { images: readonly LogResponseImage[] }) {
    const t = useTranslations('log.card');
    const [expanded, setExpanded] = useState(true);
    const panelId = useId();

    return (
        <div className="flex min-w-0 flex-col border-b border-border/70">
            <button
                type="button"
                aria-expanded={expanded}
                aria-controls={panelId}
                aria-label={expanded ? t('collapseImages') : t('expandImages')}
                onClick={() => setExpanded((value) => !value)}
                className="flex w-full min-w-0 flex-wrap items-center gap-2 px-3 py-2 text-left hover:bg-muted md:px-4"
            >
                <span className="min-w-0 text-xs font-medium text-foreground">
                    {t('generatedImages', { count: images.length })}
                </span>
                <ChevronDown className={cn('ml-auto size-4 shrink-0 text-muted-foreground', expanded && 'rotate-180')} />
            </button>
            <div
                id={panelId}
                hidden={!expanded}
                className="flex flex-col gap-3 border-t border-border/70 bg-background/20 p-3 md:p-4"
            >
                {images.map((image, index) => (
                    <GeneratedImage key={index} src={image.src} number={index + 1} />
                ))}
            </div>
        </div>
    );
}

export interface LogBodyContentProps {
    /** 已保存的正文；undefined 或空串表示尚未加载或不存在。 */
    content: string | undefined;
    /** 响应正文的保存元数据；请求侧与旧记录不提供。 */
    preview?: LogContentPreview;
    /** 后端的截断标记；配合保存元数据判定有损，缺少元数据时由调用方展示原有徽标。 */
    truncated?: boolean;
    /** 无正文时展示的占位文案。 */
    fallbackText: string;
    /** 复制/下载已保存内容时的标识；省略时不提供这两个操作。 */
    storedLabel?: string;
    /** 生成图片预览；仅响应侧传入，请求侧不展示生成结果。 */
    images?: readonly LogResponseImage[];
}

/**
 * 请求/响应/实时/历史共用的正文视图：保存元数据提示、树视图与文本视图切换，
 * 复制与下载当前正文。有损时各视图和导出内容都不代表原始全文。
 */
export function LogBodyContent({
    content,
    preview,
    truncated,
    fallbackText,
    storedLabel,
    images,
}: LogBodyContentProps) {
    const { resolvedTheme } = useTheme();
    const { isOpen } = useMorphingDialog();
    const t = useTranslations('log.card');
    const copyT = useTranslations('common.copy');
    const [shouldRender, setShouldRender] = useState(false);
    const [view, setView] = useState<BodyView>('tree');
    const [copied, setCopied] = useState(false);
    const copiedTimerRef = useRef<number | null>(null);
    const copyButtonRef = useRef<HTMLButtonElement>(null);

    const parsed = useMemo(() => {
        if (!content) return { isJson: false, structured: false, data: null as unknown };
        try {
            const data = JSON.parse(content) as unknown;
            return { isJson: true, structured: data !== null && typeof data === 'object', data };
        } catch {
            return { isJson: false, structured: false, data: null as unknown };
        }
    }, [content]);

    useEffect(() => {
        if (!isOpen) return;
        const timer = setTimeout(() => setShouldRender(true), 300);
        return () => clearTimeout(timer);
    }, [isOpen]);

    useEffect(() => () => {
        if (copiedTimerRef.current) window.clearTimeout(copiedTimerRef.current);
    }, []);

    const omissionsByPath = useMemo(() => {
        const map = new Map<string, LogContentOmission>();
        for (const omission of preview?.omissions ?? []) map.set(omission.path, omission);
        return map;
    }, [preview]);

    const handleCopy = useCallback(async () => {
        if (!content) return;
        try {
            // 正文可能很大，等待 native 写入真正完成后再标记成功，避免提前勾选。
            if (!navigator.clipboard?.writeText) throw new Error();
            await navigator.clipboard.writeText(content);
        } catch (cause) {
            // 无 native API 或写入被拒绝时退回 execCommand；仍失败则保留原始失败原因。
            if (!copyTextWithExecCommand(content, () => copyButtonRef.current?.focus())) {
                // 复制确实失败：清掉上一次的成功勾选与计时，避免误导。
                if (copiedTimerRef.current) {
                    window.clearTimeout(copiedTimerRef.current);
                    copiedTimerRef.current = null;
                }
                setCopied(false);
                toast.error(copyT('failed'), {
                    description: cause instanceof Error ? cause.message : String(cause),
                });
                return;
            }
        }
        setCopied(true);
        toast.success(copyT('success'));
        if (copiedTimerRef.current) window.clearTimeout(copiedTimerRef.current);
        copiedTimerRef.current = window.setTimeout(() => setCopied(false), 2000);
    }, [content, copyT]);

    const handleDownload = useCallback(() => {
        if (!content) return;
        try {
            const blob = new Blob([content], {
                type: parsed.isJson ? 'application/json' : 'text/plain;charset=utf-8',
            });
            const url = URL.createObjectURL(blob);
            try {
                const anchor = document.createElement('a');
                anchor.href = url;
                anchor.download = `${storedLabel ?? 'octopus-log-body'}.${parsed.isJson ? 'json' : 'txt'}`;
                document.body.appendChild(anchor);
                anchor.click();
                anchor.remove();
            } finally {
                URL.revokeObjectURL(url);
            }
        } catch (cause) {
            toast.error(t('downloadFailed'), { description: cause instanceof Error ? cause.message : String(cause) });
        }
    }, [content, parsed.isJson, storedLabel, t]);

    const activeView: BodyView = parsed.structured ? view : 'text';
    const canStore = Boolean(content) && Boolean(storedLabel);
    // 视图切换只取决于已加载的结构化内容；复制/下载才需要已保存内容标识。
    const canSwitchView = Boolean(content) && parsed.structured;

    return (
        <div className="flex min-w-0 flex-col">
            <LogLossNotice preview={preview} truncated={truncated} />
            {content && (canSwitchView || canStore) && (
                <div className="flex min-w-0 flex-wrap items-center gap-1.5 border-b border-border/70 px-3 py-1.5 md:px-4">
                    {canSwitchView && (
                        <div role="group" aria-label={t('viewMode')} className="flex shrink-0 items-center gap-1">
                            <button
                                type="button"
                                aria-pressed={activeView === 'tree'}
                                aria-label={t('viewTree')}
                                onClick={() => setView('tree')}
                                className={cn(
                                    'flex items-center gap-1 rounded-md px-1.5 py-1 text-[11px] transition-colors',
                                    activeView === 'tree'
                                        ? 'bg-primary/10 text-foreground'
                                        : 'text-muted-foreground hover:bg-muted hover:text-foreground',
                                )}
                            >
                                <Braces className="size-3.5 shrink-0" aria-hidden="true" />
                                {t('viewTree')}
                            </button>
                            <button
                                type="button"
                                aria-pressed={activeView === 'text'}
                                aria-label={t('viewText')}
                                onClick={() => setView('text')}
                                className={cn(
                                    'flex items-center gap-1 rounded-md px-1.5 py-1 text-[11px] transition-colors',
                                    activeView === 'text'
                                        ? 'bg-primary/10 text-foreground'
                                        : 'text-muted-foreground hover:bg-muted hover:text-foreground',
                                )}
                            >
                                <FileText className="size-3.5 shrink-0" aria-hidden="true" />
                                {t('viewText')}
                            </button>
                        </div>
                    )}
                    {canStore && (
                        <div className="ml-auto flex shrink-0 items-center gap-1">
                            <button
                                type="button"
                                ref={copyButtonRef}
                                onClick={() => void handleCopy()}
                                aria-label={t('copyContent')}
                                title={t('copyContent')}
                                className="flex items-center gap-1 rounded-md px-1.5 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                            >
                                <AnimatePresence mode="wait" initial={false}>
                                    {copied ? (
                                        <motion.span key="check" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
                                            <Check className="size-3.5 shrink-0" aria-hidden="true" />
                                        </motion.span>
                                    ) : (
                                        <motion.span key="copy" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
                                            <Copy className="size-3.5 shrink-0" aria-hidden="true" />
                                        </motion.span>
                                    )}
                                </AnimatePresence>
                                {t('copyContent')}
                            </button>
                            <button
                                type="button"
                                onClick={handleDownload}
                                aria-label={t('downloadContent')}
                                title={t('downloadContent')}
                                className="flex items-center gap-1 rounded-md px-1.5 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                            >
                                <Download className="size-3.5 shrink-0" aria-hidden="true" />
                                {t('downloadContent')}
                            </button>
                        </div>
                    )}
                </div>
            )}
            {shouldRender && images && images.length > 0 && <LogImagePreview images={images} />}
            {!content ? (
                <pre className="p-4 text-xs leading-relaxed whitespace-pre-wrap wrap-break-word text-muted-foreground">
                    {fallbackText}
                </pre>
            ) : (
                <AnimatePresence mode="wait">
                    {!shouldRender ? (
                        <motion.div
                            key="loading"
                            initial={{ opacity: 0 }}
                            animate={{ opacity: 1 }}
                            exit={{ opacity: 0 }}
                            transition={{ duration: 0.15 }}
                            className="flex h-full items-center justify-center p-4"
                        >
                            <Loader2 className="size-5 animate-spin text-muted-foreground" aria-hidden="true" />
                        </motion.div>
                    ) : parsed.structured && activeView === 'tree' ? (
                        <motion.div
                            key="tree"
                            initial={{ opacity: 0 }}
                            animate={{ opacity: 1 }}
                            exit={{ opacity: 0 }}
                            transition={{ duration: 0.2 }}
                            className="p-4"
                        >
                            <JsonView
                                value={parsed.data as object}
                                style={{
                                    ...(resolvedTheme === 'dark' ? githubDarkTheme : githubLightTheme),
                                    fontSize: '12px',
                                    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
                                    backgroundColor: 'transparent',
                                }}
                                displayDataTypes={false}
                                displayObjectSize={false}
                                // 长字符串按库的能力先折叠，点击字段即可展开查看已保存的完整值。
                                shortenTextAfterLength={120}
                                shouldExpandNodeInitially={(_isExpanded, { level }) => level <= initialExpandLevel}
                            >
                                <JsonView.KeyName
                                    as="span"
                                    // 树内只标注被省略的字段路径，不补占位值，避免把有损内容当作完整展示。
                                    render={(props, result) => {
                                        const omission = omissionsByPath.get(pointerFromKeys(result.keys));
                                        if (!omission) return undefined;
                                        return (
                                            <span className={props.className} style={props.style}>
                                                {props.children}
                                                <span
                                                    className="ml-1 shrink-0 rounded bg-amber-500/15 px-1 text-[10px] text-amber-700 dark:text-amber-300"
                                                    title={`${t('omittedField')} · ${t(OMISSION_KIND_KEYS[omission.kind])}`}
                                                >
                                                    {t('omitted')}
                                                </span>
                                            </span>
                                        );
                                    }}
                                />
                            </JsonView>
                        </motion.div>
                    ) : (
                        <motion.pre
                            key="text"
                            initial={{ opacity: 0 }}
                            animate={{ opacity: 1 }}
                            exit={{ opacity: 0 }}
                            transition={{ duration: 0.2 }}
                            className="p-4 font-mono text-xs leading-relaxed whitespace-pre-wrap wrap-break-word text-muted-foreground"
                        >
                            {content}
                        </motion.pre>
                    )}
                </AnimatePresence>
            )}
        </div>
    );
}
