import { useMemo, useState, useEffect } from 'react';
import { Clock, Cpu, Zap, ArrowDownToLine, ArrowUpFromLine, DollarSign, ArrowRight, ArrowDown, Send, MessageSquare, Loader2, RotateCw, Pin, KeyRound, Gauge, Square } from 'lucide-react';
import { useTranslations } from 'use-intl';
import { motion, AnimatePresence } from 'motion/react';
import JsonView from '@uiw/react-json-view';
import { githubDarkTheme } from '@uiw/react-json-view/githubDark';
import { githubLightTheme } from '@uiw/react-json-view/githubLight';
import { useTheme } from '@/provider/theme';
import { type RelayLog, type ChannelAttempt, useLogDetailStream, useLogRequestBody, useLogResponseBody, useStopAttempt } from '@/api/endpoints/log';
import { getModelIcon } from '@/lib/model-icons';
import { formatDuration } from './format';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import { CopyIconButton } from '@/components/common/CopyButton';
import {
    MorphingDialog,
    MorphingDialogTrigger,
    MorphingDialogContainer,
    MorphingDialogContent,
    MorphingDialogClose,
    MorphingDialogTitle,
    MorphingDialogDescription,
    useMorphingDialog,
} from '@/components/ui/morphing-dialog';
import { Tooltip, TooltipContent, TooltipTrigger, TooltipProvider } from '@/components/animate-ui/components/animate/tooltip';

function formatTime(timestamp: number): string {
    const date = new Date(timestamp * 1000);
    return date.toLocaleTimeString('zh-CN', {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false,
    });
}

function formatRateMultiplier(rate: number): string {
    return rate > 0 ? rate.toLocaleString('zh-CN', { maximumFractionDigits: 4 }) : '';
}

function formatCacheRate(cachedTokens: number | undefined, inputTokens: number): string {
    if (cachedTokens === undefined || inputTokens <= 0) return '—';
    return `${((cachedTokens / inputTokens) * 100).toLocaleString('zh-CN', { maximumFractionDigits: 2 })}%`;
}

function formatOutputSpeed(outputTokens: number, totalTime: number, firstTokenTime: number): string {
    const generationTime = totalTime - firstTokenTime;
    if (generationTime <= 0) return '—';
    return `${((outputTokens * 1000) / generationTime).toLocaleString('zh-CN', { maximumFractionDigits: 2 })} tokens/s`;
}
function getAttemptOrder(attempt: ChannelAttempt): number {
    const order = attempt.attempt_index ?? attempt.attempt_num;
    return Number.isFinite(order) ? order : Number.MAX_SAFE_INTEGER;
}

function getAttemptDisplayNumber(attempt: ChannelAttempt): number {
    return getAttemptOrder(attempt);
}

function sortAttempts(attempts: ChannelAttempt[]): ChannelAttempt[] {
    return attempts
        .map((attempt, index) => ({ attempt, index }))
        .sort((a, b) => getAttemptOrder(a.attempt) - getAttemptOrder(b.attempt) || a.index - b.index)
        .map(({ attempt }) => attempt);
}

interface RetryBadgeWithTooltipProps {
    channelName: string;
    brandColor: string;
    rateMultiplier: number;
    attempts: ChannelAttempt[];
}

function RetryBadgeWithTooltip({ channelName, brandColor, rateMultiplier, attempts }: RetryBadgeWithTooltipProps) {
    const t = useTranslations('log.card');

    return (
        <Tooltip>
            <TooltipTrigger asChild>
                <Badge
                    variant="secondary"
                    className="shrink-0 text-xs px-1.5 py-0 cursor-help"
                    style={{ backgroundColor: `${brandColor}15`, color: brandColor }}
                >
                    <RotateCw className="size-3 mr-1 translate-y-px opacity-80" />
                    {channelName}
                    {formatRateMultiplier(rateMultiplier) && (
                        <span className="ml-1 opacity-80">x{formatRateMultiplier(rateMultiplier)}</span>
                    )}
                </Badge>
            </TooltipTrigger>
            <TooltipContent className="border bg-card p-2 min-w-[280px] shadow-sm rounded-3xl flex flex-col gap-1">
                {attempts.map((attempt, idx) => (
                    <div key={idx} className="flex flex-col w-full">
                        <div className="flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-muted/50 transition-colors">
                            <Badge
                                className={cn(
                                    "h-5 shrink-0 px-1.5 text-[10px] font-bold uppercase shadow-none border-0",
                                    attempt.status === 'success'
                                        ? "bg-primary/15 text-primary"
                                        : "bg-destructive/15 text-destructive"
                                )}
                            >
                                {attempt.status === 'success' ? t('success') : t('failed')}
                            </Badge>
                            <div className="flex min-w-0 flex-col flex-1">
                                <span className="truncate text-xs font-semibold text-foreground">
                                    {attempt.channel_name}
                                    {formatRateMultiplier(attempt.rate_multiplier) && (
                                        <span className="ml-1 font-normal opacity-80">({t('rateMultiplier')} {formatRateMultiplier(attempt.rate_multiplier)})</span>
                                    )}
                                </span>
                                <span className="text-[10px] text-muted-foreground">
                                    {attempt.model_name} • {formatDuration(attempt.duration)}
                                </span>
                            </div>
                        </div>
                        {
                            idx < attempts.length - 1 && (
                                <div className="flex justify-center py-0.5">
                                    <ArrowDown className="size-3 text-muted-foreground/30" />
                                </div>
                            )
                        }
                    </div>
                ))}
            </TooltipContent>
        </Tooltip >
    );
}

function DeferredJsonContent({ content, fallbackText }: { content: string | undefined; fallbackText: string }) {
    const { resolvedTheme } = useTheme();
    const { isOpen } = useMorphingDialog();
    const [shouldRender, setShouldRender] = useState(false);

    const parsed = useMemo(() => {
        if (!content) return { isJson: false, data: null };
        try {
            return { isJson: true, data: JSON.parse(content) };
        } catch {
            return { isJson: false, data: content };
        }
    }, [content]);

    useEffect(() => {
        if (!isOpen) {
            setShouldRender(false);
            return;
        }
        const timer = setTimeout(() => setShouldRender(true), 300);
        return () => clearTimeout(timer);
    }, [isOpen]);

    if (!isOpen) return null;

    if (!content) {
        return (
            <pre className="p-4 text-xs text-muted-foreground whitespace-pre-wrap wrap-break-word leading-relaxed">
                {fallbackText}
            </pre>
        );
    }

    return (
        <AnimatePresence mode="wait">
            {!shouldRender ? (
                <motion.div
                    key="loading"
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                    transition={{ duration: 0.15 }}
                    className="p-4 flex items-center justify-center h-full"
                >
                    <Loader2 className="h-5 w-5 text-muted-foreground animate-spin" />
                </motion.div>
            ) : parsed.isJson ? (
                <motion.div
                    key="json"
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
                        collapsed={false}
                    />
                </motion.div>
            ) : (
                <motion.pre
                    key="text"
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                    transition={{ duration: 0.2 }}
                    className="p-4 text-xs text-muted-foreground whitespace-pre-wrap wrap-break-word font-mono leading-relaxed"
                >
                    {content}
                </motion.pre>
            )}
        </AnimatePresence>
    );
}

function LiveOverviewDetails({ log }: { log: RelayLog }) {
    const t = useTranslations('log.card');
    const statusT = useTranslations('log.status');
    const { isOpen } = useMorphingDialog();
    const detail = useLogDetailStream(log.id, log.state, isOpen);
    const stopAttempt = useStopAttempt();
    const [stopError, setStopError] = useState<string | null>(null);
    const [selectedAttemptIndex, setSelectedAttemptIndex] = useState<number | null>(null);
    const [requestExpanded, setRequestExpanded] = useState(false);
    const [now, setNow] = useState(() => Date.now());
    const isActive = log.state === 'running' || log.state === 'committed';
    const startedAt = log.started_at ? Date.parse(log.started_at) : Number.NaN;
    const liveDuration = isActive && Number.isFinite(startedAt) ? Math.max(0, now - startedAt) : log.use_time;

    useEffect(() => {
        if (!isActive) return;
        const timer = window.setInterval(() => setNow(Date.now()), 1000);
        return () => window.clearInterval(timer);
    }, [isActive]);

    const attempts = useMemo(
        () => sortAttempts(detail.attempts.length > 0 ? detail.attempts : (log.attempts ?? [])),
        [detail.attempts, log.attempts],
    );
    const runningAttempt = detail.runningAttempt ?? attempts.find((attempt) => attempt.status === 'running');

    useEffect(() => {
        if (!isOpen) {
            setSelectedAttemptIndex(null);
            setRequestExpanded(false);
            return;
        }
        setSelectedAttemptIndex((current) => {
            if (current !== null && attempts.some((attempt) => getAttemptOrder(attempt) === current)) return current;
            const lastAttempt = attempts[attempts.length - 1];
            return lastAttempt ? getAttemptOrder(lastAttempt) : null;
        });
    }, [attempts, isOpen]);

    const selectedAttempt = attempts.find((attempt) => getAttemptOrder(attempt) === selectedAttemptIndex) ?? attempts[attempts.length - 1];
    const selectedIsSuccessful = selectedAttempt?.status === 'success';
    const selectedIsCommitted = selectedAttempt?.status === 'running' && (detail.isCommitted || log.state === 'committed');
    const canShowFinalResponse = selectedAttempt ? selectedIsSuccessful || selectedIsCommitted : detail.isCommitted || log.state === 'success' || log.state === 'committed';
    const requestBody = useLogRequestBody(log.id, log.started_at, isOpen && requestExpanded);
    const responseBody = useLogResponseBody(log.id, log.started_at, isOpen && canShowFinalResponse);
    const requestContent = requestBody.data?.content ?? log.request_content;
    const responseContent = responseBody.data?.content ?? log.response_content;
    const requestTruncated = requestBody.data?.truncated ?? log.request_content_truncated;
    const responseTruncated = responseBody.data?.truncated ?? log.response_content_truncated;

    const handleStop = async () => {
        if (!runningAttempt) return;
        setStopError(null);
        try {
            await stopAttempt.mutateAsync({ requestId: log.id, attemptIndex: runningAttempt.attempt_index ?? runningAttempt.attempt_num });
        } catch (cause) {
            setStopError(cause instanceof Error ? cause.message : t('stopFailed'));
        }
    };

    const statusLabel = (status: ChannelAttempt['status']) => {
        switch (status) {
            case 'running': return statusT('running');
            case 'success': return statusT('success');
            case 'canceled': return statusT('canceled');
            case 'circuit_break': return statusT('circuitBreak');
            case 'skipped': return statusT('skipped');
            default: return statusT('failed');
        }
    };
    const selectedError = selectedAttempt && selectedAttempt.status !== 'success' ? selectedAttempt.msg || log.error : undefined;

    return (
        <div className="flex h-full min-h-0 flex-col gap-4">
            <div className="grid min-h-0 flex-1 grid-cols-1 gap-4 md:grid-cols-2">
                <div className="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-border bg-muted/30">
                    <div className="flex shrink-0 items-center gap-2 border-b border-border bg-muted/50 px-3 py-2.5 md:px-4 md:py-3">
                        <RotateCw className="size-4 text-muted-foreground" />
                        <span className="text-sm font-medium text-card-foreground">{t('attemptHistory')}</span>
                        <Badge variant="secondary" className="text-xs">{attempts.length} {t('attempts')}</Badge>
                    </div>
                    <div className="min-h-0 flex-1 overflow-auto p-2.5 md:p-3">
                        {attempts.length > 0 ? (
                            <div className="flex flex-col gap-2">
                                {attempts.map((attempt, index) => {
                                    const attemptIndex = getAttemptOrder(attempt);
                                    const selected = selectedAttempt !== undefined && attemptIndex === getAttemptOrder(selectedAttempt);
                                    return (
                                        <button key={`${attemptIndex}-${index}`} type="button" aria-pressed={selected} onClick={() => setSelectedAttemptIndex(attemptIndex)} className={cn("flex w-full flex-col gap-1.5 rounded-xl border p-2.5 text-left text-xs transition-colors", selected ? "border-primary/40 bg-primary/10 ring-1 ring-primary/20" : attempt.status === 'success' ? "border-primary/20 bg-primary/5 hover:border-primary/40" : attempt.status === 'running' ? "border-border/50 bg-secondary/30 hover:border-border" : "border-destructive/20 bg-destructive/5 hover:border-destructive/40")}>
                                            <div className="flex items-center gap-2">
                                                <span className="shrink-0 font-mono text-[10px] text-muted-foreground">{t('attemptNumber', { number: getAttemptDisplayNumber(attempt) })}</span>
                                                <Badge className={cn("border-0 px-1.5 text-[10px] font-bold uppercase", attempt.status === 'success' ? "bg-primary/15 text-primary" : attempt.status === 'running' ? "bg-secondary text-secondary-foreground" : "bg-destructive/15 text-destructive")}>{statusLabel(attempt.status)}</Badge>
                                                <span className="min-w-0 truncate font-semibold text-foreground">{attempt.channel_name}</span>
                                                {attempt.sticky && <Pin className="size-3.5 shrink-0 text-amber-500" />}
                                                {attempt.status === 'running' && <Loader2 className="ml-auto size-3.5 shrink-0 animate-spin text-muted-foreground" />}
                                                {attempt.duration > 0 && <span className="ml-auto shrink-0 tabular-nums text-muted-foreground">{formatDuration(attempt.duration)}</span>}
                                            </div>
                                            {attempt.model_name && <span className="truncate pl-0.5 text-[11px] text-muted-foreground">{attempt.model_name}</span>}
                                            <div className="flex items-center gap-2 pl-0.5 text-[11px] text-muted-foreground">{attempt.rate_multiplier > 0 && <span>x{formatRateMultiplier(attempt.rate_multiplier)}</span>}{attempt.msg && <span className="truncate text-destructive/90">{attempt.msg}</span>}</div>
                                        </button>
                                    );
                                })}
                            </div>
                        ) : isActive ? (
                            <div className="flex items-center justify-center gap-2 py-3 text-xs text-muted-foreground"><Loader2 className="size-4 animate-spin" />{t('waitingResponse')}</div>
                        ) : (
                            <div className="flex h-full items-center justify-center text-xs text-muted-foreground">{t('noAttempts')}</div>
                        )}
                    </div>
                    <button type="button" aria-expanded={requestExpanded} aria-label={t('requestContent')} onClick={() => setRequestExpanded((expanded) => !expanded)} className="flex shrink-0 items-center gap-2 border-t border-border bg-muted/50 px-3 py-2.5 text-left transition-colors hover:bg-muted md:px-4 md:py-3">
                        <Send className="size-4 text-green-500" />
                        <span className="text-sm font-medium text-card-foreground">{t('requestContent')}</span>
                        {requestTruncated && <Badge variant="outline" className="border-amber-500/40 text-xs text-amber-600 dark:text-amber-400">{t('truncated')}</Badge>}
                        <Badge variant="secondary" className="ml-auto text-xs">{log.input_tokens.toLocaleString()} {t('tokens')}</Badge>
                        <ArrowDown className={cn("size-4 shrink-0 text-muted-foreground transition-transform", requestExpanded && "rotate-180")} />
                    </button>
                    {requestExpanded && <div className="max-h-[45%] min-h-0 shrink-0 overflow-auto border-t border-border bg-background/20">{requestBody.error && !requestContent ? <div className="flex h-full items-center justify-center px-4 py-6 text-xs text-destructive">{t('detailUnavailable')}</div> : <DeferredJsonContent content={requestContent} fallbackText={t('noRequestContent')} />}</div>}
                </div>
                <section className="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-border bg-muted/30">
                    <div className="flex shrink-0 items-center gap-2 border-b border-border bg-muted/50 px-3 py-2.5 md:px-4 md:py-3">
                        <MessageSquare className="size-4 text-purple-500" />
                        <span className="text-sm font-medium text-card-foreground">{t('selectedAttempt')}</span>
                        {selectedAttempt && <Badge className={cn("border-0 px-1.5 text-[10px] font-bold uppercase", selectedAttempt.status === 'success' ? "bg-primary/15 text-primary" : selectedAttempt.status === 'running' ? "bg-secondary text-secondary-foreground" : "bg-destructive/15 text-destructive")}>{statusLabel(selectedAttempt.status)}</Badge>}
                        {runningAttempt && <button type="button" onClick={() => void handleStop()} disabled={stopAttempt.isPending} className="ml-auto flex items-center gap-1.5 rounded-md px-2 py-1 text-xs text-destructive transition-colors hover:bg-destructive/10 disabled:opacity-50">{stopAttempt.isPending ? <Loader2 className="size-3.5 animate-spin" /> : <Square className="size-3.5" />}{t('stopAttempt')}</button>}
                    </div>
                    <div className="min-h-0 flex-1 overflow-auto">
                        {stopError && <p className="border-b border-destructive/20 bg-destructive/5 px-3 py-2 text-xs text-destructive">{stopError}</p>}
                        {selectedAttempt ? <div className="flex min-h-full flex-col">
                            <div className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1 border-b border-border/70 px-3 py-3 text-xs md:px-4"><span className="font-mono text-muted-foreground">{t('attemptNumber', { number: getAttemptDisplayNumber(selectedAttempt) })}</span><span className="font-semibold text-foreground">{selectedAttempt.channel_name}</span>{selectedAttempt.model_name && <span className="text-muted-foreground">{selectedAttempt.model_name}</span>}{selectedAttempt.rate_multiplier > 0 && <span className="text-muted-foreground">x{formatRateMultiplier(selectedAttempt.rate_multiplier)}</span>}{selectedAttempt.duration > 0 && <span className="tabular-nums text-muted-foreground">{formatDuration(selectedAttempt.duration)}</span>}{selectedAttempt.sticky && <Pin className="size-3.5 text-amber-500" />}</div>
                            {selectedError && <div className="relative border-b border-destructive/20 bg-destructive/5 p-3"><CopyIconButton text={selectedError} className="absolute right-2 top-2 rounded-md p-1 text-destructive/60 transition-colors hover:bg-destructive/10 hover:text-destructive" copyIconClassName="size-4" checkIconClassName="size-4" /><p className="whitespace-pre-wrap wrap-break-word pr-8 text-sm leading-relaxed text-destructive">{selectedError}</p></div>}
                            {canShowFinalResponse ? <div className="min-h-0 flex-1">{responseTruncated && <div className="px-3 pt-3"><Badge variant="outline" className="border-amber-500/40 text-xs text-amber-600 dark:text-amber-400">{t('truncated')}</Badge></div>}{responseBody.error && !responseContent && !selectedIsCommitted ? <div className="flex h-full items-center justify-center px-4 text-xs text-destructive">{t('detailUnavailable')}</div> : <DeferredJsonContent content={responseContent} fallbackText={selectedIsCommitted ? t('responseStreaming') : t('noResponseContent')} />}</div> : selectedAttempt.status === 'running' ? <div className="flex min-h-0 flex-1 items-center justify-center gap-2 px-4 text-xs text-muted-foreground"><Loader2 className="size-4 animate-spin" />{t('waitingResponse')}</div> : !selectedError ? <div className="flex min-h-0 flex-1 items-center justify-center px-4 text-xs text-muted-foreground">{t('detailUnavailable')}</div> : null}
                        </div> : <div className="flex min-h-full flex-col">{log.error && <div className="relative border-b border-destructive/20 bg-destructive/5 p-3"><CopyIconButton text={log.error} className="absolute right-2 top-2 rounded-md p-1 text-destructive/60 transition-colors hover:bg-destructive/10 hover:text-destructive" copyIconClassName="size-4" checkIconClassName="size-4" /><p className="whitespace-pre-wrap wrap-break-word pr-8 text-sm leading-relaxed text-destructive">{log.error}</p></div>}<div className="min-h-0 flex-1">{responseBody.error && !responseContent && !isActive ? <div className="flex h-full items-center justify-center px-4 text-xs text-destructive">{t('detailUnavailable')}</div> : <DeferredJsonContent content={responseContent} fallbackText={isActive ? t('responseStreaming') : t('noResponseContent')} />}</div></div>}
                    </div>
                </section>
            </div>
            <div className="grid w-full shrink-0 grid-cols-12 gap-x-4 gap-y-2 pt-4 mt-auto text-xs text-muted-foreground md:grid-cols-7">
                <div className="col-span-4 flex items-center gap-1.5 whitespace-nowrap md:col-span-1"><Cpu className="size-3.5 shrink-0 text-blue-500" /><span>{t('totalTime')}: {formatDuration(liveDuration)}</span></div>
                <div className="col-span-4 flex items-center gap-1.5 md:col-span-1"><Zap className="size-3.5 shrink-0 text-amber-500" /><span>{t('firstTokenTime')}: {formatDuration(log.ftut)}</span></div>
                <div className="col-span-4 flex items-center gap-1.5 md:col-span-1"><Gauge className="size-3.5 shrink-0 text-rose-500" /><span>{t('outputSpeed')}: {formatOutputSpeed(log.output_tokens, log.use_time, log.ftut)}</span></div>
                <div className="col-span-3 flex items-center gap-1.5 md:col-span-1"><ArrowDownToLine className="size-3.5 shrink-0 text-cyan-500" /><span>{t('cacheRate')}: {formatCacheRate(log.cached_tokens, log.input_tokens)}</span></div>
                {log.rate_multiplier > 0 && <span className="col-span-3 flex items-center gap-1.5 md:col-span-1">{t('rateMultiplier')}: x{formatRateMultiplier(log.rate_multiplier)}</span>}
                {attempts.some((attempt) => attempt.sticky) && <span className="col-span-3 flex items-center gap-1.5 md:col-span-1"><Pin className="size-3.5 shrink-0 text-amber-500" /></span>}
            </div>
        </div>
    );
}

export function LogCard({ log }: { log: RelayLog }) {
    const t = useTranslations('log.card');
    const { Icon: ModelIcon, className: iconClassName, color: brandColor } = useMemo(
        () => getModelIcon(log.actual_model_name),
        [log.actual_model_name]
    );
    const requestAPIKeyName = useMemo(() => log.request_api_key_name?.trim() ?? '', [log.request_api_key_name]);
    const statusT = useTranslations('log.status');

    const hasError = !!log.error;
    const hasMultipleAttempts = log.attempts && log.attempts.length > 1;
    const orderedAttempts = useMemo(() => sortAttempts(log.attempts ?? []), [log.attempts]);
    return (
        <TooltipProvider>
            <MorphingDialog>
                <MorphingDialogTrigger
                    className={cn(
                        "rounded-3xl border bg-card w-full text-left",
                        hasError ? "border-destructive/40" : "border-border",
                    )}
                >
                    <div className={cn("p-4 grid grid-cols-[auto_1fr] gap-4", hasError ? "items-start" : "items-center")}>
                        <ModelIcon aria-hidden="true" className={iconClassName} width={40} height={40} />
                        <div className="min-w-0 flex flex-col gap-3">
                            <div className="flex items-center gap-2 min-w-0 text-sm">
                                <span className="font-semibold text-card-foreground truncate" title={log.request_model_name}>
                                    {log.request_model_name}
                                </span>
                                <ArrowRight className="size-3.5 shrink-0 text-muted-foreground/50" />
                                {hasMultipleAttempts ? (
                                    <RetryBadgeWithTooltip
                                        channelName={log.channel_name}
                                        brandColor={brandColor}
                                        rateMultiplier={log.rate_multiplier}
                                        attempts={orderedAttempts}
                                    />
                                ) : (
                                    <Badge
                                        variant="secondary"
                                        className="shrink-0 text-xs px-1.5 py-0"
                                        style={{ backgroundColor: `${brandColor}15`, color: brandColor }}
                                    >
                                        {log.channel_name}
                                        {formatRateMultiplier(log.rate_multiplier) && (
                                            <span className="ml-1 opacity-80">x{formatRateMultiplier(log.rate_multiplier)}</span>
                                        )}
                                    </Badge>
                                )}
                                <span className="text-muted-foreground truncate" title={log.actual_model_name}>
                                    {log.actual_model_name}
                                </span>
                                {log.is_overview && log.state && (
                                    <Badge variant="outline" className="shrink-0 text-[10px] uppercase">
                                        {statusT(log.state)}
                                    </Badge>
                                )}
                                {log.attempts?.some(a => a.sticky) && (
                                    <Pin className="size-3.5 shrink-0 text-amber-500" />
                                )}
                            </div>
                            <div className="grid grid-cols-12 gap-x-4 gap-y-2 text-xs tabular-nums text-muted-foreground [&_svg]:translate-y-px md:grid-cols-7">
                                <div className="col-span-4 flex shrink-0 items-center gap-1.5 whitespace-nowrap md:col-span-1">
                                    <Clock className="size-3.5 shrink-0" style={{ color: brandColor }} />
                                    <span>{formatTime(log.time)}</span>
                                </div>
                                {requestAPIKeyName && (
                                    <div className="col-span-4 flex min-w-0 max-w-44 items-center gap-1.5 md:col-span-1">
                                        <KeyRound className="size-3.5 shrink-0 text-orange-500" />
                                        <span className="truncate" title={requestAPIKeyName}>
                                            {requestAPIKeyName}
                                        </span>
                                    </div>
                                )}
                                <div className="col-span-4 flex shrink-0 items-center gap-1.5 whitespace-nowrap md:col-span-1">
                                    <Zap className="size-3.5 shrink-0 text-amber-500" />
                                    <span>{t('firstToken')} {formatDuration(log.ftut)}</span>
                                </div>
                                <div className="col-span-3 flex shrink-0 items-center gap-1.5 whitespace-nowrap md:col-span-1">
                                    <Cpu className="size-3.5 shrink-0 text-blue-500" />
                                    <span>{t('totalTime')} {formatDuration(log.use_time)}</span>
                                </div>
                                <div className="col-span-3 flex shrink-0 items-center gap-1.5 whitespace-nowrap md:col-span-1">
                                    <ArrowDownToLine className="size-3.5 shrink-0 text-green-500" />
                                    <span>{t('input')} {log.input_tokens.toLocaleString()}</span>
                                </div>
                                <div className="col-span-3 flex shrink-0 items-center gap-1.5 whitespace-nowrap md:col-span-1">
                                    <ArrowUpFromLine className="size-3.5 shrink-0 text-purple-500" />
                                    <span>{t('output')} {log.output_tokens.toLocaleString()}</span>
                                </div>
                                <div className="col-span-3 flex shrink-0 items-center gap-1.5 whitespace-nowrap md:col-span-1">
                                    <ArrowDownToLine className="size-3.5 shrink-0 text-cyan-500" />
                                    <span>{t('cacheTokens')} {log.cached_tokens?.toLocaleString() ?? '—'}</span>
                                </div>
                                <div className="col-span-3 flex shrink-0 items-center gap-1.5 whitespace-nowrap md:col-span-1">
                                    <ArrowDownToLine className="size-3.5 shrink-0 text-cyan-500" />
                                    <span>{t('cacheRate')} {formatCacheRate(log.cached_tokens, log.input_tokens)}</span>
                                </div>
                                <div className="col-span-3 flex shrink-0 items-center gap-1.5 whitespace-nowrap md:col-span-1">
                                    <Gauge className="size-3.5 shrink-0 text-rose-500" />
                                    <span>{t('outputSpeed')} {formatOutputSpeed(log.output_tokens, log.use_time, log.ftut)}</span>
                                </div>
                                <div className="col-span-3 flex shrink-0 items-center gap-1.5 whitespace-nowrap md:col-span-1">
                                    <DollarSign className="size-3.5 shrink-0 text-emerald-500" />
                                    <span className="font-medium text-emerald-600 dark:text-emerald-400">
                                        {t('cost')} {Number(log.cost).toFixed(6)}
                                    </span>
                                </div>
                            </div>
                            {hasError && (
                                <div className="p-2.5 rounded-xl bg-destructive/10 border border-destructive/20 overflow-hidden">
                                    <p className="text-xs text-destructive line-clamp-2">{log.error}</p>
                                </div>
                            )}
                        </div>
                    </div>
                </MorphingDialogTrigger>

                <MorphingDialogContainer>
                    <MorphingDialogContent className="relative flex h-full max-h-full w-full flex-col overflow-hidden rounded-3xl bg-card px-4 py-5 text-card-foreground sm:px-6 md:w-[80vw]">
                        <MorphingDialogClose className="top-4 right-5 text-muted-foreground hover:text-foreground transition-colors" />
                        <MorphingDialogTitle className="flex items-center gap-2 mb-4 text-sm">
                            <ModelIcon aria-hidden="true" className={iconClassName} width={28} height={28} />
                            <span className="font-semibold text-card-foreground">{log.request_model_name}</span>
                            <ArrowRight className="size-3.5 text-muted-foreground/50" />
                            {hasMultipleAttempts ? (
                                <RetryBadgeWithTooltip
                                    channelName={log.channel_name}
                                    brandColor={brandColor}
                                    rateMultiplier={log.rate_multiplier}
                                    attempts={orderedAttempts}
                                />
                            ) : (
                                <Badge
                                    variant="secondary"
                                    className="text-xs px-1.5 py-0"
                                    style={{ backgroundColor: `${brandColor}15`, color: brandColor }}
                                >
                                    {log.channel_name}
                                    {formatRateMultiplier(log.rate_multiplier) && (
                                        <span className="ml-1 opacity-80">({t('rateMultiplier')} {formatRateMultiplier(log.rate_multiplier)})</span>
                                    )}
                                </Badge>
                            )}
                            <span className="text-muted-foreground">{log.actual_model_name}</span>
                            {log.is_overview && log.state && (
                                <Badge variant="outline" className="shrink-0 text-[10px] uppercase">
                                    {statusT(log.state)}
                                </Badge>
                            )}
                            {log.attempts?.some(a => a.sticky) && (
                                <Pin className="size-3.5 shrink-0 text-amber-500" />
                            )}
                        </MorphingDialogTitle>

                        <MorphingDialogDescription className="flex-1 min-h-0">
                            {log.is_overview ? (
                                <LiveOverviewDetails log={log} />
                            ) : (
                                <div className="grid h-full min-h-0 grid-cols-1 gap-4 md:grid-cols-2">
                                    <div className="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-border bg-muted/30">
                                        <div className="flex shrink-0 items-center gap-2 border-b border-border bg-muted/50 px-3 py-2.5 md:px-4 md:py-3">
                                            <Send className="size-4 text-green-500" />
                                            <span className="text-sm font-medium text-card-foreground">{t('requestContent')}</span>
                                            {log.request_content_truncated && (
                                                <Badge variant="outline" className="border-amber-500/40 text-xs text-amber-600 dark:text-amber-400">
                                                    {t('truncated')}
                                                </Badge>
                                            )}
                                            <Badge variant="secondary" className="ml-auto text-xs">
                                                {log.input_tokens.toLocaleString()} {t('tokens')}
                                            </Badge>
                                        </div>
                                        <div className="min-h-0 flex-1 overflow-auto">
                                            <DeferredJsonContent content={log.request_content} fallbackText={t('noRequestContent')} />
                                        </div>
                                    </div>

                                    <div className="flex min-h-0 flex-col gap-4 overflow-hidden">
                                        <section className="flex min-h-0 flex-[1.1] flex-col overflow-hidden rounded-2xl border border-border bg-muted/30">
                                            <div className="flex shrink-0 items-center gap-2 border-b border-border bg-muted/50 px-3 py-2.5 md:px-4 md:py-3">
                                                <RotateCw className="size-4 text-muted-foreground" />
                                                <span className="text-sm font-medium text-card-foreground">{t('attemptHistory')}</span>
                                                <Badge variant="secondary" className="text-xs">{orderedAttempts.length} {t('attempts')}</Badge>
                                            </div>
                                            <div className="min-h-0 flex-1 overflow-auto p-2.5 md:p-3">
                                                {orderedAttempts.length > 0 ? (
                                                    <div className="flex flex-col gap-2">
                                                        {orderedAttempts.map((attempt, index) => (
                                                            <div
                                                                key={`${getAttemptOrder(attempt)}-${index}`}
                                                                className={cn(
                                                                    "flex flex-col gap-1.5 rounded-xl border p-2.5 text-xs",
                                                                    attempt.status === 'success'
                                                                        ? "border-primary/20 bg-primary/5"
                                                                        : "border-destructive/20 bg-destructive/5",
                                                                )}
                                                            >
                                                                <div className="flex items-center gap-2">
                                                                    <span className="shrink-0 font-mono text-[10px] text-muted-foreground">
                                                                        {t('attemptNumber', { number: index })}
                                                                    </span>
                                                                    <Badge
                                                                        variant="outline"
                                                                        className={cn(
                                                                            "border-0 px-1.5 text-[10px] font-bold uppercase",
                                                                            attempt.status === 'success'
                                                                                ? "bg-primary/15 text-primary"
                                                                                : attempt.status === 'running'
                                                                                    ? "bg-secondary text-secondary-foreground"
                                                                                    : "bg-destructive/15 text-destructive",
                                                                        )}
                                                                    >
                                                                        {attempt.status === 'success'
                                                                            ? statusT('success')
                                                                            : attempt.status === 'running'
                                                                                ? statusT('running')
                                                                                : attempt.status === 'canceled'
                                                                                    ? statusT('canceled')
                                                                                    : attempt.status === 'circuit_break'
                                                                                        ? statusT('circuitBreak')
                                                                                        : attempt.status === 'skipped'
                                                                                            ? statusT('skipped')
                                                                                            : statusT('failed')}
                                                                    </Badge>
                                                                    <span className="font-semibold text-foreground">{attempt.channel_name}</span>
                                                                    {attempt.model_name && <span className="truncate text-muted-foreground">({attempt.model_name})</span>}
                                                                    {attempt.sticky && <Pin className="size-3.5 text-amber-500" />}
                                                                    {attempt.rate_multiplier > 0 && <span className="text-muted-foreground">x{formatRateMultiplier(attempt.rate_multiplier)}</span>}
                                                                    {attempt.duration > 0 && <span className="ml-auto tabular-nums text-muted-foreground">{formatDuration(attempt.duration)}</span>}
                                                                </div>
                                                                {attempt.msg && (
                                                                    <div className="whitespace-pre-wrap wrap-break-word border-l-2 border-destructive/30 pl-2 text-[11px] leading-relaxed text-destructive/90">
                                                                        {attempt.msg}
                                                                    </div>
                                                                )}
                                                            </div>
                                                        ))}
                                                    </div>
                                                ) : (
                                                    <div className="flex h-full items-center justify-center text-xs text-muted-foreground">
                                                        {t('noAttempts')}
                                                    </div>
                                                )}
                                            </div>
                                        </section>

                                        <section className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-2xl border border-border bg-muted/30">
                                            <div className="flex shrink-0 items-center gap-2 border-b border-border bg-muted/50 px-3 py-2.5 md:px-4 md:py-3">
                                                <MessageSquare className="size-4 text-purple-500" />
                                                <span className="text-sm font-medium text-card-foreground">{t('finalResponse')}</span>
                                                {log.response_content_truncated && (
                                                    <Badge variant="outline" className="border-amber-500/40 text-xs text-amber-600 dark:text-amber-400">
                                                        {t('truncated')}
                                                    </Badge>
                                                )}
                                                <Badge variant="secondary" className="ml-auto text-xs">
                                                    {log.output_tokens.toLocaleString()} {t('tokens')}
                                                </Badge>
                                            </div>
                                            <div className="min-h-0 flex-1 overflow-auto">
                                                {log.error && (
                                                    <div className="relative border-b border-destructive/20 bg-destructive/5 p-3">
                                                        <CopyIconButton
                                                            text={log.error}
                                                            className="absolute right-2 top-2 rounded-md p-1 text-destructive/60 transition-colors hover:bg-destructive/10 hover:text-destructive"
                                                            copyIconClassName="size-4"
                                                            checkIconClassName="size-4"
                                                        />
                                                        <p className="whitespace-pre-wrap wrap-break-word pr-8 text-sm leading-relaxed text-destructive">
                                                            {log.error}
                                                        </p>
                                                    </div>
                                                )}
                                                <DeferredJsonContent content={log.response_content} fallbackText={t('noResponseContent')} />
                                            </div>
                                        </section>
                                    </div>
                                </div>
                            )}
                        </MorphingDialogDescription>

                        <div className="flex flex-wrap items-center gap-3 md:gap-4 pt-4 mt-auto text-xs text-muted-foreground [&_svg]:translate-y-px shrink-0">
                            <div className="flex items-center gap-1.5">
                                <Clock className="size-3.5" style={{ color: brandColor }} />
                                <span className="tabular-nums">{formatTime(log.time)}</span>
                            </div>
                            {requestAPIKeyName && (
                                <div className="flex min-w-0 items-center gap-1.5">
                                    <KeyRound className="size-3.5 shrink-0 text-orange-500" />
                                    <span className="truncate" title={requestAPIKeyName}>
                                        {requestAPIKeyName}
                                    </span>
                                </div>
                            )}
                            <div className="flex items-center gap-1.5">
                                <Zap className="size-3.5 text-amber-500" />
                                <span>{t('firstTokenTime')}: {formatDuration(log.ftut)}</span>
                            </div>
                            <div className="flex items-center gap-1.5">
                                <Cpu className="size-3.5 text-blue-500" />
                                <span>{t('totalTime')}: {formatDuration(log.use_time)}</span>
                            </div>
                            <div className="flex items-center gap-1.5">
                                <DollarSign className="size-3.5 text-emerald-500" />
                                <span className="font-medium text-emerald-600 dark:text-emerald-400">
                                    {t('cost')}: {Number(log.cost).toFixed(6)}
                                </span>
                            </div>
                        </div>
                    </MorphingDialogContent>
                </MorphingDialogContainer>
            </MorphingDialog>
        </TooltipProvider>
    );
}
