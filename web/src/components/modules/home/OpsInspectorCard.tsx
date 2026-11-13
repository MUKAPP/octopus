import { useEffect, useMemo, useState } from 'react';
import {
    AlertTriangle,
    ArrowRight,
    BarChart3,
    CheckCircle2,
    ChevronRight,
    Clock,
    KeyRound,
    Loader2,
    Radio,
    ShieldCheck,
    XCircle,
} from 'lucide-react';
import { useFormatter, useLocale, useTranslations } from 'use-intl';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/animate-ui/components/animate/tooltip';
import { useAPIKeyList } from '@/api/apikey';
import { useChannelList } from '@/api/channel';
import { useStatsAPIKey } from '@/api/stats';
import { InlineRetry } from '@/components/common/InlineRetry';
import { useAnalyticsViewStore } from '@/components/modules/analytics/store';
import { useNavStore } from '@/components/modules/navbar';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import { formatHomeMoney, inspectGatewayOps, KEY_STATUS_TTL_SEC, type OpsInspectorItem } from './home-ops';

export function OpsInspectorCard() {
    const t = useTranslations('home.inspector');
    const tHome = useTranslations('home');
    const format = useFormatter();
    const locale = useLocale();
    const { setActiveItem } = useNavStore();

    const channelQuery = useChannelList();
    const apiKeyQuery = useAPIKeyList();
    const apiKeyStatsQuery = useStatsAPIKey();

    const [nowSec, setNowSec] = useState(() => Math.floor(Date.now() / 1000));
    const lastRequestText = (timestamp: number) => timestamp > 0
        ? t('lastRequest', { time: format.relativeTime(Math.min(timestamp, nowSec) * 1000, nowSec * 1000) })
        : t('neverRequested');
    useEffect(() => {
        const timer = window.setInterval(() => {
            setNowSec(Math.floor(Date.now() / 1000));
        }, 60_000);
        return () => window.clearInterval(timer);
    }, []);

    const summary = useMemo(
        () =>
            inspectGatewayOps({
                channels: channelQuery.data,
                apiKeys: apiKeyQuery.data,
                apiKeyStats: apiKeyStatsQuery.data,
                nowSec,
            }),
        [channelQuery.data, apiKeyQuery.data, apiKeyStatsQuery.data, nowSec],
    );

    const requiredQueries = [channelQuery, apiKeyQuery, apiKeyStatsQuery];
    const isError = requiredQueries.some((query) => query.isError);
    const isLoading = !isError && requiredQueries.some((query) => query.isPending);
    const retryFailedQueries = () => Promise.allSettled(
        requiredQueries.filter((query) => query.isError).map((query) => query.refetch()),
    );

    const handleOpenAnalyticsForChannel = (id: number, name: string) => {
        useAnalyticsViewStore.setState({
            period: 'today',
            channel: { id, name },
        });
        setActiveItem('analytics');
    };

    const banner = useMemo(() => {
        if (summary.totalChannels === 0 && summary.totalApiKeys === 0) {
            return {
                tone: 'muted' as const,
                icon: ShieldCheck,
                text: t('noEntities'),
            };
        }
        if (summary.errorCount > 0 && summary.warningCount > 0) {
            return {
                tone: 'error' as const,
                icon: AlertTriangle,
                text: t('issuesSummary', { errors: summary.errorCount, warnings: summary.warningCount }),
            };
        }
        if (summary.errorCount > 0) {
            return {
                tone: 'error' as const,
                icon: XCircle,
                text: t('errorsOnlySummary', { errors: summary.errorCount }),
            };
        }
        if (summary.warningCount > 0) {
            return {
                tone: 'warning' as const,
                icon: AlertTriangle,
                text: t('warningsOnlySummary', { warnings: summary.warningCount }),
            };
        }
        if (summary.pendingCount > 0) {
            return {
                tone: 'pending' as const,
                icon: Clock,
                text: t('pendingSummary', { count: summary.pendingCount }),
            };
        }
        return {
            tone: 'healthy' as const,
            icon: CheckCircle2,
            text: t('allHealthySummary', {
                channels: summary.enabledChannels,
                keys: summary.enabledApiKeys,
            }),
        };
    }, [summary, t]);

    const bannerClass = {
        error: 'border-destructive/25 bg-destructive/10 text-destructive',
        warning: 'border-amber-500/25 bg-amber-500/10 text-amber-700 dark:text-amber-300',
        healthy: 'border-primary/25 bg-primary/10 text-primary',
        pending: 'border-slate-500/25 bg-slate-500/10 text-slate-700 dark:text-slate-300',
        muted: 'border-border bg-muted/40 text-muted-foreground',
    }[banner.tone];

    const renderItemRow = (item: OpsInspectorItem) => {
        const dotClass =
            item.severity === 'error'
                ? 'bg-destructive ring-2 ring-destructive/20'
                : item.severity === 'warning'
                    ? 'bg-amber-500 ring-2 ring-amber-500/20'
                    : item.severity === 'pending'
                        ? 'bg-slate-400 dark:bg-slate-500 ring-2 ring-slate-400/20 dark:ring-slate-500/20'
                        : 'bg-emerald-500 ring-2 ring-emerald-500/20';

        if (item.kind === 'all_channels_disabled') {
            return (
                <button
                    key={item.id}
                    type="button"
                    onClick={() => setActiveItem('channel')}
                    className="group flex w-full items-center gap-2 rounded-2xl px-3 py-2.5 text-left transition-colors motion-reduce:transition-none hover:bg-muted/50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                >
                    <div className="flex min-w-0 flex-1 items-center gap-2.5">
                        <span className={cn('size-2 rounded-full shrink-0', dotClass)} aria-hidden="true" />
                        <span className="min-w-0 flex-1 truncate text-sm font-medium text-card-foreground">
                            {t('allChannelsDisabled', { total: item.totalChannelCount })}
                        </span>
                    </div>
                    <span
                        aria-hidden="true"
                        className="inline-flex size-7 shrink-0 items-center justify-center rounded-lg text-muted-foreground/70 transition-colors motion-reduce:transition-none group-hover:text-foreground"
                    >
                        <ChevronRight className="size-3.5" />
                    </span>
                </button>
            );
        }

        if (item.kind === 'apikey') {
            const quotaText =
                item.maxCost !== undefined && item.maxCost > 0
                    ? `${formatHomeMoney(item.usedCost).text} / ${formatHomeMoney(item.maxCost).text}`
                    : null;
            const barWidth = Math.min(100, Math.max(0, item.quotaPercent ?? 0));

            let detailText = '';
            if (item.reason === 'expired') {
                detailText = t('apiKeyExpired');
            } else if (item.reason === 'quota_exhausted') {
                detailText = t('apiKeyQuotaExhausted', { percent: item.quotaPercent ?? 100 });
            } else if (item.reason === 'quota_near') {
                detailText = t('apiKeyQuotaNear', { percent: item.quotaPercent ?? 80 });
            } else {
                detailText =
                    item.daysLeft !== undefined && item.daysLeft >= 1
                        ? t('apiKeyExpiringDays', { days: item.daysLeft })
                        : t('apiKeyExpiringSoon');
            }

            return (
                <button
                    key={item.id}
                    type="button"
                    onClick={() => setActiveItem('setting')}
                    className="group flex w-full items-center gap-2 rounded-2xl px-3 py-2.5 text-left transition-colors motion-reduce:transition-none hover:bg-muted/50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                >
                    <div className="flex min-w-0 flex-1 items-center gap-2.5">
                        <span className={cn('size-2 rounded-full shrink-0', dotClass)} aria-hidden="true" />
                        <KeyRound className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
                        <div className="min-w-0 flex-1">
                            <div className="flex items-center justify-between gap-2">
                                <span className="truncate text-sm font-medium text-card-foreground">
                                    {t('apiKeyLabel', { name: item.apiKey.name })}
                                </span>
                                <span
                                    className={cn(
                                        'shrink-0 text-xs font-medium tabular-nums',
                                        item.severity === 'error'
                                            ? 'text-destructive'
                                            : 'text-amber-600 dark:text-amber-400',
                                    )}
                                >
                                    {detailText}
                                </span>
                            </div>
                            {quotaText && (
                                <div className="mt-1.5 flex items-center gap-2">
                                    <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
                                        <div
                                            className={cn(
                                                'h-full rounded-full transition-all motion-reduce:transition-none',
                                                item.severity === 'error' ? 'bg-destructive' : 'bg-amber-500',
                                            )}
                                            style={{ width: `${barWidth}%` }}
                                        />
                                    </div>
                                    <span className="shrink-0 text-[11px] tabular-nums text-muted-foreground">
                                        {quotaText}
                                    </span>
                                </div>
                            )}
                        </div>
                    </div>
                    <span
                        aria-hidden="true"
                        className="inline-flex size-7 shrink-0 items-center justify-center rounded-lg text-muted-foreground/70 transition-colors motion-reduce:transition-none group-hover:text-foreground"
                    >
                        <ChevronRight className="size-3.5" />
                    </span>
                </button>
            );
        }

        const delayClass =
            item.bestDelayMs <= 0
                ? 'text-muted-foreground'
                : item.bestDelayMs < 300
                    ? 'text-emerald-600 dark:text-emerald-400'
                    : item.bestDelayMs < 1000
                        ? 'text-muted-foreground'
                        : 'text-amber-600 dark:text-amber-400';

        return (
            <div
                key={item.id}
                className="group flex items-center gap-2 rounded-2xl px-3 py-2 transition-colors motion-reduce:transition-none hover:bg-muted/50"
            >
                <Tooltip>
                    <TooltipTrigger asChild>
                        <button
                            type="button"
                            onClick={() => setActiveItem('channel')}
                            className="flex min-w-0 flex-1 items-start gap-2.5 rounded-lg text-left focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                        >
                            <span className={cn('size-2 rounded-full shrink-0 mt-1.5', dotClass)} aria-hidden="true" />
                            <span className="min-w-0 flex-1">
                                <span className="block truncate text-sm font-medium text-card-foreground">{item.channel.name}</span>
                                <span className="flex items-center gap-1 truncate text-[11px] text-muted-foreground mt-0.5">
                                    <Clock className="size-2.5 shrink-0 opacity-60" aria-hidden="true" />
                                    <span className="truncate">{lastRequestText(item.lastUsedAt)}</span>
                                </span>
                            </span>

                            <div className="ml-auto flex shrink-0 items-center gap-2 self-center">
                                {item.reason === 'no_enabled_keys' && (
                                    <Badge variant="outline" className="h-5 border-destructive/35 bg-destructive/10 px-1.5 text-[11px] font-medium text-destructive">
                                        {t('noEnabledChannelKeys')}
                                    </Badge>
                                )}
                                {(item.reason === 'key_error' || item.reason === 'key_rate_limit') && (
                                    <Badge
                                        variant="outline"
                                        className={cn(
                                            'h-5 max-w-36 sm:max-w-44 truncate px-1.5 text-[11px] font-medium tabular-nums',
                                            item.severity === 'error'
                                                ? 'border-destructive/35 bg-destructive/10 text-destructive'
                                                : 'border-amber-500/35 bg-amber-500/10 text-amber-700 dark:text-amber-300',
                                        )}
                                    >
                                        {t('channelKeyStatus', {
                                            label: item.keyLabel ?? '#1',
                                            code: item.statusCode ?? 0,
                                        })}
                                    </Badge>
                                )}
                                {item.reason === 'high_delay' && (
                                    <Badge variant="outline" className="h-5 border-amber-500/35 bg-amber-500/10 px-1.5 text-[11px] font-medium text-amber-700 dark:text-amber-300">
                                        {t('highLatency')}
                                    </Badge>
                                )}
                                {item.reason === 'pending' && (
                                    <Badge
                                        variant="outline"
                                        className="h-5 gap-1 rounded-full border-slate-500/25 bg-slate-500/10 px-2 text-[11px] font-medium text-slate-700 dark:text-slate-300"
                                    >
                                        <Clock className="size-2.5 shrink-0 opacity-70" aria-hidden="true" />
                                        <span>{t('pendingKeys', { count: item.pendingKeyCount })}</span>
                                    </Badge>
                                )}
                                {item.reason === 'healthy' && (
                                    <span className="text-xs text-muted-foreground tabular-nums">
                                        {t('allKeysHealthy', {
                                            enabled: item.enabledKeyCount,
                                            total: item.totalKeyCount,
                                        })}
                                    </span>
                                )}

                                {item.bestDelayMs > 0 && (
                                    <span className={cn('w-12 text-right text-xs font-medium tabular-nums', delayClass)}>
                                        {item.bestDelayMs}ms
                                    </span>
                                )}
                            </div>
                        </button>
                    </TooltipTrigger>
                    {/* Tooltip 原语跳过 children 比较，用版本属性同步数据、时间与语言变化。 */}
                    <TooltipContent
                        data-scrollable-tooltip
                        data-content-version={`${channelQuery.dataUpdatedAt}:${nowSec}:${locale}`}
                        innerClassName="p-0"
                        className="w-[min(24rem,calc(100vw-2rem))] rounded-2xl border border-border/70 bg-popover/95 p-3.5 sm:p-4 text-popover-foreground shadow-2xl backdrop-blur-md"
                        arrowClassName="fill-popover"
                    >
                        <div data-scrollable-tooltip className="scrollbar max-h-[min(24rem,calc(100dvh-6rem))] w-full space-y-3 overflow-y-auto overscroll-contain text-left pr-0.5">
                            <div className="flex items-center justify-between gap-3 border-b border-border/40 pb-2.5">
                                <div className="flex items-center gap-2 min-w-0">
                                    <div className="flex size-6 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                                        <Radio className="size-3.5" aria-hidden="true" />
                                    </div>
                                    <span className="font-semibold text-sm text-foreground truncate">{item.channel.name}</span>
                                </div>
                                <Badge variant="outline" className="h-5 rounded-full px-2 text-[10px] font-medium text-muted-foreground shrink-0 border-border/60 bg-muted/40">
                                    {t('allKeysHealthy', { enabled: item.enabledKeyCount, total: item.totalKeyCount })}
                                </Badge>
                            </div>

                            <div className="flex items-start gap-2 rounded-xl border border-border/40 bg-muted/40 px-3 py-2 text-[11px] leading-relaxed text-muted-foreground">
                                <Clock className="size-3.5 shrink-0 mt-0.5 opacity-70" aria-hidden="true" />
                                <span>{t('statusWindow')}</span>
                            </div>

                            <div className="space-y-2 pt-0.5">
                                {item.channel.keys.map((key, index) => {
                                    if (!key.enabled) return null;
                                    const isRecent = key.last_use_time_stamp > 0
                                        && nowSec - key.last_use_time_stamp <= KEY_STATUS_TTL_SEC
                                        && key.status_code !== 0;
                                    const isNever = key.last_use_time_stamp <= 0;
                                    const keyLabel = key.remark.trim() || `#${index + 1}`;

                                    let badgeNode: React.ReactNode;
                                    let statusDesc: string;

                                    if (isRecent) {
                                        if (key.status_code === 200) {
                                            badgeNode = (
                                                <Badge variant="outline" className="h-5 rounded-full px-2 text-[10px] font-medium tabular-nums border-emerald-500/30 bg-emerald-500/12 text-emerald-700 dark:text-emerald-400">
                                                    {t('keyStatusOk', { code: 200 })}
                                                </Badge>
                                            );
                                        } else if (key.status_code === 429) {
                                            badgeNode = (
                                                <Badge variant="outline" className="h-5 rounded-full px-2 text-[10px] font-medium tabular-nums border-amber-500/30 bg-amber-500/12 text-amber-700 dark:text-amber-300">
                                                    {t('keyStatusRateLimit', { code: 429 })}
                                                </Badge>
                                            );
                                        } else {
                                            badgeNode = (
                                                <Badge variant="outline" className="h-5 rounded-full px-2 text-[10px] font-medium tabular-nums border-destructive/30 bg-destructive/12 text-destructive">
                                                    {t('keyStatusError', { code: key.status_code })}
                                                </Badge>
                                            );
                                        }
                                        statusDesc = lastRequestText(key.last_use_time_stamp);
                                    } else {
                                        badgeNode = (
                                            <Badge variant="outline" className="h-5 rounded-full px-2 text-[10px] font-medium border-border/80 bg-muted/60 text-muted-foreground">
                                                {tHome('pulse.statusPending')}
                                            </Badge>
                                        );
                                        if (isNever || key.status_code === 0) {
                                            statusDesc = t('neverRequested');
                                        } else {
                                            statusDesc = `${t('keyStaleStatus', { status: key.status_code })} · ${lastRequestText(key.last_use_time_stamp)}`;
                                        }
                                    }

                                    return (
                                        <div
                                            key={key.id}
                                            className="flex flex-col gap-1.5 rounded-xl border border-border/40 bg-card/60 p-2.5 sm:p-3 text-xs transition-colors hover:bg-muted/30"
                                        >
                                            <div className="flex items-center justify-between gap-2">
                                                <div className="flex items-center gap-1.5 min-w-0">
                                                    <KeyRound className="size-3.5 shrink-0 text-muted-foreground/80" aria-hidden="true" />
                                                    <span className="font-medium text-card-foreground truncate max-w-[150px] sm:max-w-[180px]">
                                                        {keyLabel}
                                                    </span>
                                                </div>
                                                {badgeNode}
                                            </div>
                                            <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                                                <Clock className="size-3 shrink-0 opacity-60" aria-hidden="true" />
                                                <span className="truncate">{statusDesc}</span>
                                            </div>
                                        </div>
                                    );
                                })}
                            </div>
                        </div>
                    </TooltipContent>
                </Tooltip>

                <button
                    type="button"
                    onClick={() => handleOpenAnalyticsForChannel(item.channel.id, item.channel.name)}
                    aria-label={t('viewChannelAnalytics', { name: item.channel.name })}
                    title={t('viewChannelAnalytics', { name: item.channel.name })}
                    className="inline-flex size-7 shrink-0 items-center justify-center rounded-lg text-muted-foreground/70 transition-colors motion-reduce:transition-none hover:bg-muted hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                >
                    <BarChart3 className="size-3.5" aria-hidden="true" />
                </button>
            </div>
        );
    };

    const BannerIcon = banner.icon;

    return (
        <section className="flex min-w-0 flex-col rounded-3xl border border-border bg-card p-4 sm:p-5 text-card-foreground">
            <div className="mb-3 flex items-center justify-between gap-2">
                <h2 className="text-base font-semibold truncate">{t('title')}</h2>
                <button
                    type="button"
                    onClick={() => setActiveItem('channel')}
                    className="inline-flex shrink-0 items-center gap-1 rounded-lg px-2 py-1 text-xs font-medium text-muted-foreground transition-colors motion-reduce:transition-none hover:bg-muted/50 hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                >
                    <span>{t('manage')}</span>
                    <ArrowRight className="size-3.5" aria-hidden="true" />
                </button>
            </div>

            {isLoading ? (
                <div className="flex flex-1 min-h-48 items-center justify-center">
                    <Loader2 className="size-7 animate-spin motion-reduce:animate-none text-muted-foreground" role="status" aria-label={tHome('feedback.loading')} />
                </div>
            ) : isError ? (
                <div role="alert" className="flex flex-1 min-h-48 flex-col items-center justify-center gap-3 text-center text-sm text-muted-foreground">
                    <p>{tHome('feedback.loadFailed')}</p>
                    <InlineRetry
                        onRetry={retryFailedQueries}
                        isRetrying={requiredQueries.some((query) => query.isFetching)}
                        label={tHome('feedback.retry')}
                    />
                </div>
            ) : (
                <div className="flex flex-1 flex-col gap-3 min-h-0">
                    <div className={cn('flex items-center gap-2 rounded-2xl border px-3.5 py-2.5 text-xs font-medium', bannerClass)}>
                        <BannerIcon className="size-4 shrink-0" aria-hidden="true" />
                        <span className="min-w-0 truncate">{banner.text}</span>
                    </div>

                    <div className="scrollbar flex-1 max-h-[252px] min-h-[160px] space-y-1 overflow-y-auto overscroll-contain">
                        {summary.items.length === 0 ? (
                            <div className="flex h-40 flex-col items-center justify-center gap-2 text-center text-muted-foreground">
                                <ShieldCheck className="size-8 opacity-35" aria-hidden="true" />
                                <p className="text-xs">{t('noEntities')}</p>
                            </div>
                        ) : (
                            <TooltipProvider openDelay={200} closeDelay={150}>
                                {summary.items.map(renderItemRow)}
                            </TooltipProvider>
                        )}
                    </div>
                </div>
            )}
        </section>
    );
}
