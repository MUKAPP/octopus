import { useMemo } from 'react';
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts';
import { ArrowRight, Loader2 } from 'lucide-react';
import { useTranslations } from 'use-intl';
import { useStatsHourly, useStatsToday } from '@/api/stats';
import { AnimatedNumber } from '@/components/common/AnimatedNumber';
import { InlineRetry } from '@/components/common/InlineRetry';
import { Tabs, TabsList, TabsTrigger } from '@/components/animate-ui/components/animate/tabs';
import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart';
import { useNavStore } from '@/components/modules/navbar';
import { formatCount } from '@/lib/utils';
import { formatHomeMoney } from './home-ops';
import { useHomeViewStore, type ChartMetricType } from './store';

interface HourlyPoint {
    hourLabel: string;
    value: number | null;
}

export function HourlyPulseChart() {
    const t = useTranslations('home.chart');
    const tHome = useTranslations('home');
    const { setActiveItem } = useNavStore();

    const hourlyQuery = useStatsHourly();
    const todayQuery = useStatsToday();
    const { data: statsHourly } = hourlyQuery;
    const today = todayQuery.data;
    const chartQueries = [hourlyQuery, todayQuery];
    const retryFailedQueries = () => Promise.allSettled(
        chartQueries.filter((query) => query.isError).map((query) => query.refetch()),
    );

    const chartMetricType = useHomeViewStore((state) => state.chartMetricType);
    const setChartMetricType = useHomeViewStore((state) => state.setChartMetricType);

    const { points, nonNullCount } = useMemo(() => {
        const byHour = new Map((statsHourly ?? []).filter((item) => item.date === today?.date).map((item) => [item.hour, item]));
        const built: HourlyPoint[] = [];
        let populated = 0;

        for (let hour = 0; hour < 24; hour++) {
            const stat = byHour.get(hour);
            const hourLabel = `${String(hour).padStart(2, '0')}:00`;
            if (!stat) {
                built.push({ hourLabel, value: null });
                continue;
            }
            populated += 1;
            const value =
                chartMetricType === 'cost'
                    ? stat.total_cost.raw
                    : chartMetricType === 'count'
                        ? stat.request_count.raw
                        : stat.total_token.raw;
            built.push({ hourLabel, value });
        }

        return { points: built, nonNullCount: populated };
    }, [statsHourly, chartMetricType, today?.date]);

    // 小时分布只覆盖有请求明细的样本，汇总始终沿用与指标卡相同的完整日聚合。
    const totals = {
        requests: (today?.request_success ?? 0) + (today?.request_failed ?? 0),
        cost: (today?.input_cost ?? 0) + (today?.output_cost ?? 0),
        tokens: (today?.input_token ?? 0) + (today?.output_token ?? 0),
    };
    const hourlySampleCount = (statsHourly ?? [])
        .filter((item) => item.date === today?.date)
        .reduce((count, item) => count + item.request_count.raw, 0);

    const chartConfig = useMemo(() => {
        const label =
            chartMetricType === 'cost'
                ? t('totalCost')
                : chartMetricType === 'count'
                    ? t('totalRequests')
                    : t('totalTokens');
        return {
            value: { label },
        };
    }, [chartMetricType, t]);

    const formatMetricAxis = (val: number) => {
        if (chartMetricType === 'cost') {
            if (!Number.isFinite(val) || val <= 0 || val < 0.0001) return '$0';
            if (val < 0.1) return `$${Number(val.toFixed(4))}`;
            return formatHomeMoney(val).text;
        }
        const formatted = formatCount(val).formatted;
        return `${formatted.value}${formatted.unit}`;
    };

    const formatMetricTooltip = (val: number) => {
        if (chartMetricType === 'cost') {
            if (val >= 0.01 && val < 0.1) return `$${Number(val.toFixed(4))}`;
            return formatHomeMoney(val).text;
        }
        const formatted = formatCount(val).formatted;
        return `${formatted.value}${formatted.unit}`;
    };

    const strokeColor =
        chartMetricType === 'cost'
            ? 'var(--chart-1)'
            : chartMetricType === 'count'
                ? 'var(--chart-2)'
                : 'var(--chart-3)';

    const gradientId = `homeHourlyPulseFill-${chartMetricType}`;

    if (chartQueries.some((query) => query.isError && !query.data)) {
        return (
            <div role="alert" className="flex h-64 flex-col items-center justify-center gap-3 rounded-3xl border border-destructive/30 bg-card p-4 text-center text-sm text-muted-foreground">
                <p>{tHome('feedback.loadFailed')}</p>
                <InlineRetry onRetry={retryFailedQueries} label={tHome('feedback.retry')} isRetrying={chartQueries.some((query) => query.isFetching)} />
            </div>
        );
    }

    if (!statsHourly || !today) {
        return (
            <div className="flex h-64 items-center justify-center rounded-3xl border border-border bg-card text-muted-foreground">
                <Loader2 className="size-8 animate-spin motion-reduce:animate-none" role="status" aria-label={tHome('feedback.loading')} />
            </div>
        );
    }

    const totalRequestsFormatted = formatCount(totals.requests).formatted;
    const totalTokensFormatted = formatCount(totals.tokens).formatted;
    const totalCostDisplay = formatHomeMoney(totals.cost);

    return (
        <section className="rounded-3xl border border-border bg-card pt-4 pb-2 text-card-foreground">
            {chartQueries.some((query) => query.isError) && (
                <div role="alert" className="mx-4 mb-3 flex items-center justify-between gap-3 rounded-2xl border border-destructive/30 bg-destructive/5 px-4 py-2.5 text-sm text-muted-foreground">
                    <span>{tHome('feedback.loadFailed')}</span>
                    <InlineRetry onRetry={retryFailedQueries} label={tHome('feedback.retry')} className="shrink-0" isRetrying={chartQueries.some((query) => query.isFetching)} />
                </div>
            )}

            <div className="space-y-3 px-4 sm:px-5 pb-2">
                <div className="flex flex-wrap items-center justify-between gap-3">
                    <h2 className="min-w-0 truncate text-base font-semibold">{t('title')}</h2>

                    <div className="flex flex-wrap items-center gap-2">
                        <Tabs value={chartMetricType} onValueChange={(value) => setChartMetricType(value as ChartMetricType)}>
                            <TabsList>
                                <TabsTrigger value="cost">{t('metricType.cost')}</TabsTrigger>
                                <TabsTrigger value="count">{t('metricType.count')}</TabsTrigger>
                                <TabsTrigger value="tokens">{t('metricType.tokens')}</TabsTrigger>
                            </TabsList>
                        </Tabs>

                        <button
                            type="button"
                            onClick={() => setActiveItem('analytics')}
                            className="inline-flex h-10 items-center gap-1 rounded-2xl border border-border/70 bg-muted/40 px-3.5 text-sm font-medium text-foreground transition-colors motion-reduce:transition-none hover:bg-muted/70 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                        >
                            <span>{t('deepAnalytics')}</span>
                            <ArrowRight className="size-3.5 text-muted-foreground" aria-hidden="true" />
                        </button>
                    </div>
                </div>

                {/* 今日汇总数字栏 */}
                <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-sm tabular-nums">
                    <div className="flex items-baseline gap-1.5">
                        <span className="text-xs text-muted-foreground">{t('totalRequests')}</span>
                        <span className="font-semibold">
                            <AnimatedNumber value={totalRequestsFormatted.value} />
                            {totalRequestsFormatted.unit && (
                                <span className="ml-0.5 text-xs font-normal text-muted-foreground">{totalRequestsFormatted.unit}</span>
                            )}
                        </span>
                    </div>
                    <div className="h-3.5 w-px bg-border" aria-hidden="true" />
                    <div className="flex items-baseline gap-1.5">
                        <span className="text-xs text-muted-foreground">{t('totalCost')}</span>
                        <span className="font-semibold">
                            {totalCostDisplay.animValue !== undefined ? (
                                <>
                                    {totalCostDisplay.prefixDollar && <span className="mr-0.5 font-normal text-muted-foreground">$</span>}
                                    <AnimatedNumber value={totalCostDisplay.animValue} />
                                    {totalCostDisplay.unitSuffix && (
                                        <span className="ml-0.5 text-xs font-normal text-muted-foreground">{totalCostDisplay.unitSuffix}</span>
                                    )}
                                </>
                            ) : (
                                totalCostDisplay.text
                            )}
                        </span>
                    </div>
                    <div className="h-3.5 w-px bg-border" aria-hidden="true" />
                    <div className="flex items-baseline gap-1.5">
                        <span className="text-xs text-muted-foreground">{t('totalTokens')}</span>
                        <span className="font-semibold">
                            <AnimatedNumber value={totalTokensFormatted.value} />
                            {totalTokensFormatted.unit && (
                                <span className="ml-0.5 text-xs font-normal text-muted-foreground">{totalTokensFormatted.unit}</span>
                            )}
                        </span>
                    </div>
                </div>
            </div>

            {hourlySampleCount < totals.requests && (
                <p className="px-4 pb-2 text-xs text-muted-foreground sm:px-5">
                    {t('hourlyCoverage', {
                        sampled: formatCount(hourlySampleCount).formatted.value + formatCount(hourlySampleCount).formatted.unit,
                        total: totalRequestsFormatted.value + totalRequestsFormatted.unit,
                    })}
                </p>
            )}

            <ChartContainer config={chartConfig} className="h-44 w-full">
                <AreaChart accessibilityLayer data={points} margin={{ top: 8, left: 4, right: 24, bottom: 0 }}>
                    <defs>
                        <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
                            <stop offset="5%" stopColor={strokeColor} stopOpacity={0.55} />
                            <stop offset="95%" stopColor={strokeColor} stopOpacity={0.03} />
                        </linearGradient>
                    </defs>
                    <CartesianGrid strokeDasharray="3 3" vertical={false} />
                    <XAxis
                        dataKey="hourLabel"
                        tickLine={false}
                        axisLine={false}
                        minTickGap={28}
                    />
                    <YAxis
                        tickLine={false}
                        axisLine={false}
                        width={58}
                        tickFormatter={(val) => formatMetricAxis(Number(val))}
                    />
                    <ChartTooltip
                        cursor={false}
                        content={
                            <ChartTooltipContent
                                indicator="line"
                                formatter={(value) => (
                                    <span className="font-mono font-medium tabular-nums">
                                        {formatMetricTooltip(Number(value))}
                                    </span>
                                )}
                            />
                        }
                    />
                    <Area
                        type="monotone"
                        dataKey="value"
                        stroke={strokeColor}
                        strokeWidth={2}
                        fill={`url(#${gradientId})`}
                        connectNulls={false}
                        dot={nonNullCount <= 1 ? { r: 3, fill: strokeColor, strokeWidth: 0 } : false}
                        isAnimationActive={false}
                    />
                </AreaChart>
            </ChartContainer>
        </section>
    );
}
