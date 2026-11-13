import { useEffect, useMemo, useState } from 'react';
import { motion, useReducedMotion } from 'motion/react';
import { Activity, DollarSign, Loader2, Radio, Zap } from 'lucide-react';
import { useLocale, useTranslations } from 'use-intl';
import { useAPIKeyList } from '@/api/apikey';
import { useChannelList } from '@/api/channel';
import { useModelList } from '@/api/model';
import { useStatsAPIKey, useStatsToday, useStatsTotal } from '@/api/stats';
import { AnimatedNumber } from '@/components/common/AnimatedNumber';
import { InlineRetry } from '@/components/common/InlineRetry';
import { Badge } from '@/components/ui/badge';
import { DURATION, EASING, REDUCED_MOTION_TRANSITION } from '@/lib/animations/fluid-transitions';
import { cn, formatCount } from '@/lib/utils';
import { formatDuration } from '@/components/modules/log/format';
import {
    formatExactOrCompactCount,
    formatHomeMoney,
    inspectGatewayOps,
    isPricedModel,
} from './home-ops';

interface PulseCardsProps {
    activeCount: number;
}

export function PulseCards({ activeCount }: PulseCardsProps) {
    const t = useTranslations('home.pulse');
    const tHome = useTranslations('home');
    const locale = useLocale();
    const shouldReduceMotion = useReducedMotion() ?? false;

    const todayQuery = useStatsToday();
    const totalQuery = useStatsTotal();
    const channelQuery = useChannelList();
    const apiKeyQuery = useAPIKeyList();
    const apiKeyStatsQuery = useStatsAPIKey();
    const modelQuery = useModelList();

    const [nowSec, setNowSec] = useState(() => Math.floor(Date.now() / 1000));
    useEffect(() => {
        const timer = window.setInterval(() => {
            setNowSec(Math.floor(Date.now() / 1000));
        }, 60_000);
        return () => window.clearInterval(timer);
    }, []);

    const opsSummary = useMemo(
        () =>
            inspectGatewayOps({
                channels: channelQuery.data,
                apiKeys: apiKeyQuery.data,
                apiKeyStats: apiKeyStatsQuery.data,
                nowSec,
            }),
        [channelQuery.data, apiKeyQuery.data, apiKeyStatsQuery.data, nowSec],
    );

    const pricedModelsCount = useMemo(
        () => (modelQuery.data ?? []).filter(isPricedModel).length,
        [modelQuery.data],
    );

    const today = todayQuery.data;
    const isInitialLoading = todayQuery.isLoading && !today;
    const gatewayQueries = [channelQuery, apiKeyQuery, apiKeyStatsQuery, modelQuery];
    const gatewayHasError = gatewayQueries.some((query) => query.isError);
    const gatewayIsLoading = gatewayQueries.some((query) => query.isPending);
    const retryGatewayQueries = () => Promise.allSettled(
        gatewayQueries.filter((query) => query.isError).map((query) => query.refetch()),
    );

    if (isInitialLoading) {
        return (
            <div className="grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4">
                <div className="col-span-full flex min-h-32 items-center justify-center rounded-3xl border border-border bg-card text-muted-foreground">
                    <Loader2 className="size-7 animate-spin motion-reduce:animate-none" role="status" aria-label={tHome('feedback.loading')} />
                </div>
            </div>
        );
    }

    if (todayQuery.isError && !today) {
        return (
            <div className="grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4">
                <div role="alert" className="col-span-full flex min-h-32 flex-col items-center justify-center gap-3 rounded-3xl border border-destructive/30 bg-card p-4 text-center text-sm text-muted-foreground">
                    <p>{tHome('feedback.loadFailed')}</p>
                    <InlineRetry onRetry={() => void todayQuery.refetch()} label={tHome('feedback.retry')} />
                </div>
            </div>
        );
    }

    const successCount = today?.request_success ?? 0;
    const failedCount = today?.request_failed ?? 0;
    const totalRequestsToday = successCount + failedCount;
    const successRate = totalRequestsToday > 0 ? (successCount / totalRequestsToday) * 100 : null;
    const requestsFormatted = formatCount(totalRequestsToday).formatted;

    const todayCost = (today?.input_cost ?? 0) + (today?.output_cost ?? 0);
    const todayCostDisplay = formatHomeMoney(todayCost);
    const todayTokens = (today?.input_token ?? 0) + (today?.output_token ?? 0);
    const todayTokensCompact = formatCount(todayTokens).formatted;
    const allTimeCostDisplay = formatHomeMoney(totalQuery.data?.total_cost.raw);

    const avgLatencyMs = totalRequestsToday > 0 && today ? Math.round(today.wait_time / totalRequestsToday) : null;

    const hasChannelError = opsSummary.errorCount > 0 || opsSummary.warningCount > 0;
    const gatewayBadge = opsSummary.enabledChannels === 0
        ? {
            label: t('statusInactive'),
            className: 'bg-muted text-muted-foreground border-border',
        }
        : hasChannelError
            ? {
                label: t('statusAttention'),
                className: 'bg-amber-500/15 text-amber-700 dark:text-amber-400 border-amber-500/30',
            }
            : opsSummary.pendingCount > 0
                ? {
                    label: t('statusPending'),
                    className: 'bg-muted text-muted-foreground border-border',
                }
            : {
                label: t('statusHealthy'),
                className: 'bg-primary/15 text-primary border-primary/25',
            };

    const rateBadgeClass = successRate === null
        ? 'bg-muted text-muted-foreground border-border'
        : successRate >= 95
            ? 'bg-primary/15 text-primary border-primary/25'
            : successRate >= 80
                ? 'bg-amber-500/15 text-amber-700 dark:text-amber-400 border-amber-500/30'
                : 'bg-destructive/15 text-destructive border-destructive/30';

    const cardVariants = shouldReduceMotion
        ? { initial: { opacity: 0 }, animate: { opacity: 1 } }
        : { initial: { opacity: 0, y: 12, filter: 'blur(8px)' }, animate: { opacity: 1, y: 0, filter: 'blur(0px)' } };

    return (
        <div className="grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4">
            {todayQuery.isError && today && (
                <div role="alert" className="col-span-full flex items-center justify-between gap-3 rounded-2xl border border-destructive/30 bg-destructive/5 px-4 py-2.5 text-sm text-muted-foreground">
                    <span>{tHome('feedback.loadFailed')}</span>
                    <InlineRetry onRetry={() => void todayQuery.refetch()} label={tHome('feedback.retry')} className="shrink-0" />
                </div>
            )}

            {/* 1. 今日请求 */}
            <motion.section
                className="flex min-w-0 flex-col justify-between gap-2 rounded-3xl border border-border bg-card p-4 sm:p-5 text-card-foreground"
                initial={cardVariants.initial}
                animate={cardVariants.animate}
                transition={shouldReduceMotion ? REDUCED_MOTION_TRANSITION : { duration: DURATION.page, ease: EASING.easeOutExpo, delay: 0 }}
            >
                <div className="flex items-center justify-between gap-2">
                    <span className="truncate text-xs font-medium text-muted-foreground">{t('todayRequests')}</span>
                    <Activity className="size-4 shrink-0 text-primary/75" aria-hidden="true" />
                </div>
                <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                    <span className="text-2xl sm:text-[26px] font-bold tracking-tight tabular-nums leading-none">
                        <AnimatedNumber value={requestsFormatted.value} />
                        {requestsFormatted.unit && (
                            <span className="ml-0.5 text-sm font-medium text-muted-foreground">{requestsFormatted.unit}</span>
                        )}
                    </span>
                    {successRate !== null && (
                        <Badge variant="outline" className={cn('h-5 px-1.5 text-[11px] font-medium tabular-nums', rateBadgeClass)}>
                            {successRate.toFixed(1)}%
                        </Badge>
                    )}
                </div>
                <div className="truncate text-xs text-muted-foreground tabular-nums">
                    {totalRequestsToday === 0 ? (
                        <span>{t('noRequestsToday')}</span>
                    ) : (
                        <>
                            <span>{t('success', { count: formatExactOrCompactCount(successCount, locale) })}</span>
                            <span className="mx-1.5 text-border">·</span>
                            <span className={failedCount > 0 ? 'font-medium text-destructive' : undefined}>
                                {t('failed', { count: formatExactOrCompactCount(failedCount, locale) })}
                            </span>
                        </>
                    )}
                </div>
            </motion.section>

            {/* 2. 今日消耗 */}
            <motion.section
                className="flex min-w-0 flex-col justify-between gap-2 rounded-3xl border border-border bg-card p-4 sm:p-5 text-card-foreground"
                initial={cardVariants.initial}
                animate={cardVariants.animate}
                transition={shouldReduceMotion ? REDUCED_MOTION_TRANSITION : { duration: DURATION.page, ease: EASING.easeOutExpo, delay: 0.04 }}
            >
                <div className="flex items-center justify-between gap-2">
                    <span className="truncate text-xs font-medium text-muted-foreground">{t('todaySpend')}</span>
                    <DollarSign className="size-4 shrink-0 text-chart-2" aria-hidden="true" />
                </div>
                <div className="flex items-baseline gap-1 min-w-0">
                    {todayCostDisplay.animValue !== undefined ? (
                        <span className="text-2xl sm:text-[26px] font-bold tracking-tight tabular-nums leading-none">
                            {todayCostDisplay.prefixDollar && <span className="mr-0.5 font-semibold text-foreground/85">$</span>}
                            <AnimatedNumber value={todayCostDisplay.animValue} />
                            {todayCostDisplay.unitSuffix && (
                                <span className="ml-0.5 text-sm font-medium text-muted-foreground">{todayCostDisplay.unitSuffix}</span>
                            )}
                        </span>
                    ) : (
                        <span className="text-2xl sm:text-[26px] font-bold tracking-tight tabular-nums leading-none">
                            {todayCostDisplay.text}
                        </span>
                    )}
                </div>
                <div className="truncate text-xs text-muted-foreground tabular-nums">
                    <span>{t('todayTokens', { value: `${todayTokensCompact.value}${todayTokensCompact.unit}` })}</span>
                    {totalQuery.data && (
                        <>
                            <span className="mx-1.5 text-border">·</span>
                            <span>{t('allTimeCost', { value: allTimeCostDisplay.text })}</span>
                        </>
                    )}
                </div>
            </motion.section>

            {/* 3. 实时并发 */}
            <motion.section
                className="flex min-w-0 flex-col justify-between gap-2 rounded-3xl border border-border bg-card p-4 sm:p-5 text-card-foreground"
                initial={cardVariants.initial}
                animate={cardVariants.animate}
                transition={shouldReduceMotion ? REDUCED_MOTION_TRANSITION : { duration: DURATION.page, ease: EASING.easeOutExpo, delay: 0.08 }}
            >
                <div className="flex items-center justify-between gap-2">
                    <span className="truncate text-xs font-medium text-muted-foreground">{t('liveConcurrency')}</span>
                    <Zap className="size-4 shrink-0 text-chart-3" aria-hidden="true" />
                </div>
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                    <div className="flex items-baseline gap-1">
                        <span className="text-2xl sm:text-[26px] font-bold tracking-tight tabular-nums leading-none">
                            <AnimatedNumber value={activeCount} />
                        </span>
                        <span className="text-xs sm:text-sm font-medium text-muted-foreground">{t('concurrencyUnit')}</span>
                    </div>
                    <Badge
                        variant="outline"
                        className={cn(
                            'h-5 px-1.5 text-[11px] font-medium gap-1',
                            activeCount > 0
                                ? 'bg-primary/15 text-primary border-primary/25'
                                : 'bg-muted text-muted-foreground border-border',
                        )}
                    >
                        {activeCount > 0 && (
                            <span className="size-1.5 rounded-full bg-primary animate-pulse motion-reduce:animate-none" aria-hidden="true" />
                        )}
                        {activeCount > 0 ? t('running') : t('idle')}
                    </Badge>
                </div>
                <div className="truncate text-xs text-muted-foreground tabular-nums">
                    {avgLatencyMs !== null
                        ? t('avgLatencyToday', { value: formatDuration(avgLatencyMs) })
                        : activeCount > 0
                            ? t('processingLatency')
                            : t('noLatencyToday')}
                </div>
            </motion.section>

            {/* 4. 网关状态 */}
            <motion.section
                className="flex min-w-0 flex-col justify-between gap-2 rounded-3xl border border-border bg-card p-4 sm:p-5 text-card-foreground"
                initial={cardVariants.initial}
                animate={cardVariants.animate}
                transition={shouldReduceMotion ? REDUCED_MOTION_TRANSITION : { duration: DURATION.page, ease: EASING.easeOutExpo, delay: 0.12 }}
            >
                <div className="flex items-center justify-between gap-2">
                    <span className="truncate text-xs font-medium text-muted-foreground">{t('gatewayStatus')}</span>
                    <Radio className="size-4 shrink-0 text-chart-4" aria-hidden="true" />
                </div>
                {gatewayHasError ? (
                    <div role="alert" className="flex flex-1 flex-col items-start justify-center gap-2 text-xs text-muted-foreground">
                        <span>{tHome('feedback.loadFailed')}</span>
                        <InlineRetry
                            onRetry={retryGatewayQueries}
                            isRetrying={gatewayQueries.some((query) => query.isFetching)}
                            label={tHome('feedback.retry')}
                        />
                    </div>
                ) : gatewayIsLoading ? (
                    <div className="flex flex-1 items-center justify-center py-3 text-muted-foreground">
                        <Loader2 className="size-5 animate-spin motion-reduce:animate-none" role="status" aria-label={tHome('feedback.loading')} />
                    </div>
                ) : (
                    <>
                        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                            <span className="text-2xl sm:text-[26px] font-bold tracking-tight tabular-nums leading-none">
                                <AnimatedNumber value={opsSummary.enabledChannels} />
                                <span className="mx-1 text-base sm:text-lg font-normal text-muted-foreground">/</span>
                                <span className="text-lg sm:text-xl font-semibold text-muted-foreground">{opsSummary.totalChannels}</span>
                            </span>
                            <Badge variant="outline" className={cn('h-5 px-1.5 text-[11px] font-medium', gatewayBadge.className)}>
                                {gatewayBadge.label}
                            </Badge>
                        </div>
                        <div className="truncate text-xs text-muted-foreground tabular-nums">
                            <span>{t('activeKeys', { count: opsSummary.enabledApiKeys })}</span>
                            {modelQuery.data && (
                                <>
                                    <span className="mx-1.5 text-border">·</span>
                                    <span>{t('pricedModels', { count: pricedModelsCount })}</span>
                                </>
                            )}
                        </div>
                    </>
                )}
            </motion.section>
        </div>
    );
}
