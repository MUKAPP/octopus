import { useMemo, useState, useEffect, useRef } from 'react';
import { Clock, Cpu, Zap, ArrowDownToLine, ArrowUpFromLine, DollarSign, ArrowRight, ArrowDown, Send, MessageSquare, Loader2, RotateCw, Pin, KeyRound, Gauge, Square, ChevronDown } from 'lucide-react';
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
    const order = getAttemptOrder(attempt);
    return attempt.attempt_index !== undefined && attempt.attempt_num > attempt.attempt_index ? order + 1 : order;
}

function sortAttempts(attempts: ChannelAttempt[]): ChannelAttempt[] {
    return attempts
        .map((attempt, index) => ({ attempt, index }))
        .sort((a, b) => getAttemptOrder(a.attempt) - getAttemptOrder(b.attempt) || a.index - b.index)
        .map(({ attempt }) => attempt);
}
type AttemptStatusTranslationKey = 'running' | 'success' | 'canceled' | 'circuitBreak' | 'skipped' | 'failed';

function getAttemptStatusLabelKey(status: ChannelAttempt['status']): AttemptStatusTranslationKey {
    switch (status) {
        case 'running': return 'running';
        case 'success': return 'success';
        case 'canceled': return 'canceled';
        case 'circuit_break': return 'circuitBreak';
        case 'skipped': return 'skipped';
        default: return 'failed';
    }
}

function getAttemptStatusClass(status: ChannelAttempt['status']): string {
    switch (status) {
        case 'success': return 'bg-primary/15 text-primary';
        case 'running': return 'bg-secondary text-secondary-foreground';
        case 'canceled': return 'bg-amber-500/15 text-amber-700 dark:text-amber-300';
        case 'circuit_break': return 'bg-orange-500/15 text-orange-700 dark:text-orange-300';
        case 'skipped': return 'bg-muted text-muted-foreground';
        default: return 'bg-destructive/15 text-destructive';
    }
}


interface RetryBadgeWithTooltipProps {
    channelName: string;
    brandColor: string;
    rateMultiplier: number;
    attempts: ChannelAttempt[];
}

function RetryBadgeWithTooltip({ channelName, brandColor, rateMultiplier, attempts }: RetryBadgeWithTooltipProps) {
    const t = useTranslations('log.card');
    const statusT = useTranslations('log.status');

    return (
        <Tooltip>
            <TooltipTrigger asChild>
                <Badge
                    variant="secondary"
                    className="shrink-0 cursor-help px-1.5 py-0 text-xs"
                    style={{ backgroundColor: `${brandColor}15`, color: brandColor }}
                >
                    <RotateCw className="mr-1 size-3 translate-y-px opacity-80" />
                    {channelName}
                    {formatRateMultiplier(rateMultiplier) && (
                        <span className="ml-1 opacity-80">x{formatRateMultiplier(rateMultiplier)}</span>
                    )}
                </Badge>
            </TooltipTrigger>
            <TooltipContent className="w-[min(22rem,calc(100vw-2rem))] min-w-0 rounded-3xl border bg-card p-2 shadow-sm">
                <div className="flex flex-col gap-1">
                    {attempts.map((attempt, idx) => (
                        <div key={idx} className="flex w-full flex-col">
                            <div className="flex min-w-0 items-center gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-muted/50">
                                <Badge className={cn("h-5 shrink-0 border-0 px-1.5 text-[10px] font-bold uppercase shadow-none", getAttemptStatusClass(attempt.status))}>
                                    {statusT(getAttemptStatusLabelKey(attempt.status))}
                                </Badge>
                                <div className="flex min-w-0 flex-1 flex-col">
                                    <span className="truncate text-xs font-semibold text-foreground">
                                        {attempt.channel_name}
                                        {formatRateMultiplier(attempt.rate_multiplier) && (
                                            <span className="ml-1 font-normal opacity-80">({t('rateMultiplier')} {formatRateMultiplier(attempt.rate_multiplier)})</span>
                                        )}
                                    </span>
                                    <span className="truncate text-[10px] text-muted-foreground">
                                        {attempt.model_name} • {formatDuration(attempt.duration)}
                                    </span>
                                </div>
                            </div>
                            {idx < attempts.length - 1 && (
                                <div className="flex justify-center py-0.5">
                                    <ArrowDown className="size-3 text-muted-foreground/30" />
                                </div>
                            )}
                        </div>
                    ))}
                </div>
            </TooltipContent>
        </Tooltip>
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
function LogMetrics({ log, brandColor, totalTime = log.use_time }: { log: RelayLog; brandColor: string; totalTime?: number }) {
    const t = useTranslations('log.card');
    const requestAPIKeyName = log.request_api_key_name?.trim() ?? '';
    const timeLabel = formatTime(log.time);
    const firstTokenLabel = `${t('firstTokenTime')}: ${formatDuration(log.ftut)}`;
    const totalTimeLabel = `${t('totalTime')}: ${formatDuration(totalTime)}`;
    const speedLabel = `${t('outputSpeed')}: ${formatOutputSpeed(log.output_tokens, totalTime, log.ftut)}`;
    const cacheRateLabel = `${t('cacheRate')}: ${formatCacheRate(log.cached_tokens, log.input_tokens)}`;
    const costLabel = `${t('cost')}: ${Number(log.cost).toFixed(6)}`;
    const rateLabel = `x${formatRateMultiplier(log.rate_multiplier)}`;

    return (
        <div className="mt-auto grid w-full shrink-0 grid-cols-1 gap-x-3 gap-y-2 pt-4 text-xs leading-5 text-muted-foreground sm:grid-cols-3 lg:grid-cols-5 min-[420px]:grid-cols-2 [&_svg]:translate-y-px">
            <div className="flex min-w-0 items-center gap-1.5">
                <Clock className="size-3.5 shrink-0" style={{ color: brandColor }} />
                <span className="min-w-0 truncate tabular-nums" title={timeLabel}>{timeLabel}</span>
            </div>
            {requestAPIKeyName && (
                <div className="flex min-w-0 items-center gap-1.5">
                    <KeyRound className="size-3.5 shrink-0 text-orange-500" />
                    <span className="min-w-0 truncate" title={requestAPIKeyName}>{requestAPIKeyName}</span>
                </div>
            )}
            <div className="flex min-w-0 items-center gap-1.5">
                <Zap className="size-3.5 shrink-0 text-amber-500" />
                <span className="min-w-0 truncate" title={firstTokenLabel}>{firstTokenLabel}</span>
            </div>
            <div className="flex min-w-0 items-center gap-1.5">
                <Cpu className="size-3.5 shrink-0 text-blue-500" />
                <span className="min-w-0 truncate" title={totalTimeLabel}>{totalTimeLabel}</span>
            </div>
            <div className="flex min-w-0 items-center gap-1.5">
                <Gauge className="size-3.5 shrink-0 text-rose-500" />
                <span className="min-w-0 truncate" title={speedLabel}>{speedLabel}</span>
            </div>
            <div className="flex min-w-0 items-center gap-1.5">
                <ArrowDownToLine className="size-3.5 shrink-0 text-cyan-500" />
                <span className="min-w-0 truncate" title={cacheRateLabel}>{cacheRateLabel}</span>
            </div>
            {log.rate_multiplier > 0 && (
                <div className="flex min-w-0 items-center gap-1.5">
                    <span className="min-w-0 truncate" title={rateLabel}>{t('rateMultiplier')}: {rateLabel}</span>
                </div>
            )}
            <div className="flex min-w-0 items-center gap-1.5">
                <DollarSign className="size-3.5 shrink-0 text-emerald-500" />
                <span className="min-w-0 truncate font-medium text-emerald-600 dark:text-emerald-400" title={costLabel}>{costLabel}</span>
            </div>
            {log.attempts?.some((attempt) => attempt.sticky) && <Pin className="size-3.5 shrink-0 text-amber-500" />}
        </div>
    );
}


function LiveOverviewDetails({ log, brandColor }: { log: RelayLog; brandColor: string }) {
    const t = useTranslations('log.card');
    const statusT = useTranslations('log.status');
    const { isOpen } = useMorphingDialog();
    const detail = useLogDetailStream(log.id, log.state, isOpen);
    const stopAttempt = useStopAttempt();
    const [stopError, setStopError] = useState<string | null>(null);
    const [selectedAttemptIndex, setSelectedAttemptIndex] = useState<number | null>(null);
    const manualAttemptSelectionRef = useRef(false);
    const [requestExpanded, setRequestExpanded] = useState(false);
    const [attemptHistoryExpanded, setAttemptHistoryExpanded] = useState(true);
    const [responseExpanded, setResponseExpanded] = useState(true);
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
            manualAttemptSelectionRef.current = false;
            setSelectedAttemptIndex(null);
            setRequestExpanded(false);
            setAttemptHistoryExpanded(true);
            setResponseExpanded(true);
            return;
        }
        setSelectedAttemptIndex((current) => {
            const currentAttempt = current === null
                ? undefined
                : attempts.find((attempt) => getAttemptOrder(attempt) === current);
            if (manualAttemptSelectionRef.current && currentAttempt) return current;
            const lastSuccessfulAttempt = [...attempts].reverse().find((attempt) => attempt.status === 'success');
            const preferredAttempt = lastSuccessfulAttempt ?? attempts[attempts.length - 1];
            return preferredAttempt ? getAttemptOrder(preferredAttempt) : null;
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
    const previousResponseReadyRef = useRef(false);
    const previousLogStateRef = useRef(log.state);
    useEffect(() => {
        const previousState = previousLogStateRef.current;
        previousLogStateRef.current = log.state;
        const responseReady = isOpen && canShowFinalResponse;
        if (!responseReady) {
            previousResponseReadyRef.current = false;
            return;
        }
        const becameReady = !previousResponseReadyRef.current;
        const becameTerminal = previousState !== log.state
            && (log.state === 'success' || log.state === 'failed' || log.state === 'canceled');
        previousResponseReadyRef.current = true;
        if (becameReady || becameTerminal) {
            void responseBody.refetch();
        }
    }, [canShowFinalResponse, isOpen, log.state, responseBody.refetch]);
    const responseLoading = responseBody.isLoading || (responseBody.isFetching && !responseContent);

    const handleStop = async () => {
        if (!runningAttempt) return;
        setStopError(null);
        try {
            await stopAttempt.mutateAsync({ requestId: log.id, attemptIndex: runningAttempt.attempt_index ?? runningAttempt.attempt_num });
        } catch (cause) {
            setStopError(cause instanceof Error ? cause.message : t('stopFailed'));
        }
    };
    const handleAttemptSelect = (attemptIndex: number) => {
        manualAttemptSelectionRef.current = true;
        setSelectedAttemptIndex(attemptIndex);
    };
    const handleRequestToggle = () => {
        if (requestExpanded) {
            setRequestExpanded(false);
            setResponseExpanded(true);
        } else {
            setRequestExpanded(true);
            setResponseExpanded(false);
        }
    };
    const handleResponseToggle = () => {
        if (responseExpanded) {
            setResponseExpanded(false);
            setRequestExpanded(true);
        } else {
            setResponseExpanded(true);
            setRequestExpanded(false);
        }
    };

    const selectedError = selectedAttempt && selectedAttempt.status !== 'success' ? selectedAttempt.msg || log.error : undefined;

    return (
        <div className="flex h-full min-h-0 flex-col gap-4">
            <div className="grid min-h-0 flex-1 grid-cols-1 gap-4 md:grid-cols-2">
                <div className="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-border bg-muted/30">
                    <div className={cn(
                        "flex min-h-0 flex-col overflow-hidden",
                        requestExpanded ? "hidden" : "md:flex-1",
                        attemptHistoryExpanded ? "flex-1" : "shrink-0",
                    )}>
                        <div className="flex shrink-0 items-center gap-2 border-b border-border bg-muted/50 px-3 py-2.5 md:px-4 md:py-3">
                            <RotateCw className="size-4 text-muted-foreground" />
                            <span className="text-sm font-medium text-card-foreground">{t('attemptHistory')}</span>
                            <Badge variant="secondary" className="text-xs">{attempts.length} {t('attempts')}</Badge>
                            <button
                                type="button"
                                aria-label={t('attemptHistory')}
                                aria-expanded={attemptHistoryExpanded}
                                aria-controls={`attempt-history-${log.id}`}
                                onClick={() => setAttemptHistoryExpanded((expanded) => !expanded)}
                                className="ml-auto rounded-md p-1 text-muted-foreground transition-colors hover:bg-muted md:hidden"
                            >
                                <ChevronDown className={cn("size-4 transition-transform", attemptHistoryExpanded && "rotate-180")} />
                            </button>
                        </div>
                        <div
                            id={`attempt-history-${log.id}`}
                            className={cn("min-h-0 flex-1 overflow-auto p-2.5 md:p-3", !attemptHistoryExpanded && "hidden md:block")}
                        >
                            {attempts.length > 0 ? (
                                <div className="flex flex-col gap-2">
                                    {attempts.map((attempt, index) => {
                                        const attemptIndex = getAttemptOrder(attempt);
                                        const selected = selectedAttempt !== undefined && attemptIndex === getAttemptOrder(selectedAttempt);
                                        return (
                                            <button key={`${attemptIndex}-${index}`} type="button" aria-pressed={selected} onClick={() => handleAttemptSelect(attemptIndex)} className={cn("flex w-full flex-col gap-1.5 rounded-xl border p-2.5 text-left text-xs transition-colors", selected ? "border-primary/40 bg-primary/10 ring-1 ring-primary/20" : attempt.status === 'success' ? "border-primary/20 bg-primary/5 hover:border-primary/40" : attempt.status === 'running' ? "border-border/50 bg-secondary/30 hover:border-border" : "border-destructive/20 bg-destructive/5 hover:border-destructive/40")}>
                                                <div className="flex items-center gap-2">
                                                    <span className="shrink-0 font-mono text-[10px] text-muted-foreground">{t('attemptNumber', { number: getAttemptDisplayNumber(attempt) })}</span>
                                                    <Badge className={cn("border-0 px-1.5 text-[10px] font-bold uppercase", getAttemptStatusClass(attempt.status))}>{statusT(getAttemptStatusLabelKey(attempt.status))}</Badge>
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
                    </div>
                    <button type="button" aria-expanded={requestExpanded} aria-label={t('requestContent')} onClick={handleRequestToggle} className="flex shrink-0 items-center gap-2 border-t border-border bg-muted/50 px-3 py-2.5 text-left transition-colors hover:bg-muted md:px-4 md:py-3">
                        <Send className="size-4 text-green-500" />
                        <span className="text-sm font-medium text-card-foreground">{t('requestContent')}</span>
                        {requestTruncated && <Badge variant="outline" className="border-amber-500/40 text-xs text-amber-600 dark:text-amber-400">{t('truncated')}</Badge>}
                        <Badge variant="secondary" className="ml-auto text-xs">{log.input_tokens.toLocaleString()} {t('tokens')}</Badge>
                        <ArrowDown className={cn("size-4 shrink-0 text-muted-foreground transition-transform", requestExpanded && "rotate-180")} />
                    </button>
                    {requestExpanded && <div className="min-h-0 flex-1 overflow-auto border-t border-border bg-background/20">{requestBody.error && !requestContent ? <div className="flex h-full items-center justify-center px-4 py-6 text-xs text-destructive">{t('detailUnavailable')}</div> : <DeferredJsonContent content={requestContent} fallbackText={t('noRequestContent')} />}</div>}
                </div>
                <section className="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-border bg-muted/30">
                    <div className="flex shrink-0 items-center gap-2 border-b border-border bg-muted/50 px-3 py-2.5 md:px-4 md:py-3">
                        <MessageSquare className="size-4 text-purple-500" />
                        <span className="text-sm font-medium text-card-foreground">{t('selectedAttempt')}</span>
                        {selectedAttempt && <Badge className={cn("border-0 px-1.5 text-[10px] font-bold uppercase", getAttemptStatusClass(selectedAttempt.status))}>{statusT(getAttemptStatusLabelKey(selectedAttempt.status))}</Badge>}
                        <button
                            type="button"
                            aria-label={t('finalResponse')}
                            aria-expanded={responseExpanded}
                            aria-controls={`response-content-${log.id}`}
                            onClick={handleResponseToggle}
                            className={cn("rounded-md p-1 text-muted-foreground transition-colors hover:bg-muted md:hidden", !runningAttempt && "ml-auto")}
                        >
                            <ChevronDown className={cn("size-4 transition-transform", responseExpanded && "rotate-180")} />
                        </button>
                        {runningAttempt && <button type="button" onClick={() => void handleStop()} disabled={stopAttempt.isPending} className="ml-auto flex items-center gap-1.5 rounded-md px-2 py-1 text-xs text-destructive transition-colors hover:bg-destructive/10 disabled:opacity-50">{stopAttempt.isPending ? <Loader2 className="size-3.5 animate-spin" /> : <Square className="size-3.5" />}{t('stopAttempt')}</button>}
                    </div>
                    <div className="min-h-0 flex-1 overflow-auto">
                        {stopError && <p className="border-b border-destructive/20 bg-destructive/5 px-3 py-2 text-xs text-destructive">{stopError}</p>}
                        {selectedAttempt ? <div className="flex min-h-full flex-col">
                            <div className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1 border-b border-border/70 px-3 py-3 text-xs md:px-4">
                                <span className="font-mono text-muted-foreground">{t('attemptNumber', { number: getAttemptDisplayNumber(selectedAttempt) })}</span>
                                {selectedAttempt.model_name && <span className="text-muted-foreground">{selectedAttempt.model_name}</span>}
                                {selectedAttempt.rate_multiplier > 0 && <span className="text-muted-foreground">x{formatRateMultiplier(selectedAttempt.rate_multiplier)}</span>}
                                {selectedAttempt.duration > 0 && <span className="tabular-nums text-muted-foreground">{formatDuration(selectedAttempt.duration)}</span>}
                            </div>
                            <div id={`response-content-${log.id}`} className={cn("min-h-0 flex-1", !responseExpanded && "hidden md:block")}>
                                {selectedError && <div className="relative border-b border-destructive/20 bg-destructive/5 p-3"><CopyIconButton text={selectedError} className="absolute right-2 top-2 rounded-md p-1 text-destructive/60 transition-colors hover:bg-destructive/10 hover:text-destructive" copyIconClassName="size-4" checkIconClassName="size-4" /><p className="whitespace-pre-wrap wrap-break-word pr-8 text-sm leading-relaxed text-destructive">{selectedError}</p></div>}
                                {canShowFinalResponse ? <div className="min-h-0 flex-1">{responseTruncated && <div className="px-3 pt-3"><Badge variant="outline" className="border-amber-500/40 text-xs text-amber-600 dark:text-amber-400">{t('truncated')}</Badge></div>}{responseLoading ? <div className="flex h-full items-center justify-center"><Loader2 className="size-5 animate-spin text-muted-foreground" /></div> : responseBody.error && !responseContent && !selectedIsCommitted ? <div className="flex h-full items-center justify-center px-4 text-xs text-destructive">{t('detailUnavailable')}</div> : <DeferredJsonContent content={responseContent} fallbackText={selectedIsCommitted ? t('responseStreaming') : t('noResponseContent')} />}</div> : selectedAttempt.status === 'running' ? <div className="flex min-h-0 flex-1 items-center justify-center gap-2 px-4 text-xs text-muted-foreground"><Loader2 className="size-4 animate-spin" />{t('waitingResponse')}</div> : !selectedError ? <div className="flex min-h-0 flex-1 items-center justify-center px-4 text-xs text-muted-foreground">{t('detailUnavailable')}</div> : null}
                            </div>
                        </div> : <div className="flex min-h-full flex-col">
                            {log.error && <div className="relative border-b border-destructive/20 bg-destructive/5 p-3"><CopyIconButton text={log.error} className="absolute right-2 top-2 rounded-md p-1 text-destructive/60 transition-colors hover:bg-destructive/10 hover:text-destructive" copyIconClassName="size-4" checkIconClassName="size-4" /><p className="whitespace-pre-wrap wrap-break-word pr-8 text-sm leading-relaxed text-destructive">{log.error}</p></div>}
                            <div id={`response-content-${log.id}`} className={cn("min-h-0 flex-1", !responseExpanded && "hidden md:block")}>
                                <DeferredJsonContent content={responseContent} fallbackText={isActive ? t('responseStreaming') : t('noResponseContent')} />
                            </div>
                        </div>}
                    </div>
                </section>
            </div>
            <LogMetrics log={log} brandColor={brandColor} totalTime={liveDuration} />
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
                            <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-sm">
                                <span className="min-w-0 break-words font-semibold text-card-foreground" title={log.request_model_name}>
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
                                <span className="min-w-0 break-words text-muted-foreground" title={log.actual_model_name}>
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
                            <div className="grid grid-cols-2 gap-x-3 gap-y-2 text-xs leading-5 tabular-nums text-muted-foreground sm:grid-cols-3 lg:grid-cols-5 [&_svg]:translate-y-px">
                                <div className="flex min-w-0 items-center gap-1.5">
                                    <Clock className="size-3.5 shrink-0" style={{ color: brandColor }} />
                                    <span className="min-w-0 truncate" title={formatTime(log.time)}>{formatTime(log.time)}</span>
                                </div>
                                {requestAPIKeyName && (
                                    <div className="flex min-w-0 items-center gap-1.5">
                                        <KeyRound className="size-3.5 shrink-0 text-orange-500" />
                                        <span className="min-w-0 truncate" title={requestAPIKeyName}>{requestAPIKeyName}</span>
                                    </div>
                                )}
                                <div className="flex min-w-0 items-center gap-1.5">
                                    <Zap className="size-3.5 shrink-0 text-amber-500" />
                                    <span className="min-w-0 truncate" title={`${t('firstToken')} ${formatDuration(log.ftut)}`}>{t('firstToken')} {formatDuration(log.ftut)}</span>
                                </div>
                                <div className="flex min-w-0 items-center gap-1.5">
                                    <Cpu className="size-3.5 shrink-0 text-blue-500" />
                                    <span className="min-w-0 truncate" title={`${t('totalTime')} ${formatDuration(log.use_time)}`}>{t('totalTime')} {formatDuration(log.use_time)}</span>
                                </div>
                                <div className="flex min-w-0 items-center gap-1.5">
                                    <ArrowDownToLine className="size-3.5 shrink-0 text-green-500" />
                                    <span className="min-w-0 truncate" title={`${t('input')} ${log.input_tokens.toLocaleString()}`}>{t('input')} {log.input_tokens.toLocaleString()}</span>
                                </div>
                                <div className="flex min-w-0 items-center gap-1.5">
                                    <ArrowUpFromLine className="size-3.5 shrink-0 text-purple-500" />
                                    <span className="min-w-0 truncate" title={`${t('output')} ${log.output_tokens.toLocaleString()}`}>{t('output')} {log.output_tokens.toLocaleString()}</span>
                                </div>
                                <div className="flex min-w-0 items-center gap-1.5">
                                    <ArrowDownToLine className="size-3.5 shrink-0 text-cyan-500" />
                                    <span className="min-w-0 truncate" title={`${t('cacheTokens')} ${log.cached_tokens?.toLocaleString() ?? '—'}`}>{t('cacheTokens')} {log.cached_tokens?.toLocaleString() ?? '—'}</span>
                                </div>
                                <div className="flex min-w-0 items-center gap-1.5">
                                    <ArrowDownToLine className="size-3.5 shrink-0 text-cyan-500" />
                                    <span className="min-w-0 truncate" title={`${t('cacheRate')} ${formatCacheRate(log.cached_tokens, log.input_tokens)}`}>{t('cacheRate')} {formatCacheRate(log.cached_tokens, log.input_tokens)}</span>
                                </div>
                                <div className="flex min-w-0 items-center gap-1.5">
                                    <Gauge className="size-3.5 shrink-0 text-rose-500" />
                                    <span className="min-w-0 truncate" title={`${t('outputSpeed')} ${formatOutputSpeed(log.output_tokens, log.use_time, log.ftut)}`}>{t('outputSpeed')} {formatOutputSpeed(log.output_tokens, log.use_time, log.ftut)}</span>
                                </div>
                                <div className="flex min-w-0 items-center gap-1.5">
                                    <DollarSign className="size-3.5 shrink-0 text-emerald-500" />
                                    <span className="min-w-0 truncate font-medium text-emerald-600 dark:text-emerald-400" title={`${t('cost')} ${Number(log.cost).toFixed(6)}`}>{t('cost')} {Number(log.cost).toFixed(6)}</span>
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
                                <LiveOverviewDetails log={log} brandColor={brandColor} />
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
                                                                        {t('attemptNumber', { number: index + 1 })}
                                                                    </span>
                                                                    <Badge
                                                                        variant="outline"
                                                                        className={cn(
                                                                            "border-0 px-1.5 text-[10px] font-bold uppercase",
                                                                            getAttemptStatusClass(attempt.status),
                                                                        )}
                                                                    >
                                                                        {statusT(getAttemptStatusLabelKey(attempt.status))}
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

                        {!log.is_overview && <LogMetrics log={log} brandColor={brandColor} />}
                    </MorphingDialogContent>
                </MorphingDialogContainer>
            </MorphingDialog>
        </TooltipProvider>
    );
}
