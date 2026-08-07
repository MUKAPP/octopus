import { useMemo } from 'react';
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts';
import { useTranslations } from 'use-intl';
import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart';
import { formatCount, formatMoney } from '@/lib/utils';
import { Tabs, TabsList, TabsTrigger } from '@/components/animate-ui/components/animate/tabs';
import { Loader2, TrendingUp } from 'lucide-react';
import type { UseQueryResult } from '@tanstack/react-query';
import type { AnalyticsResponseFormatted } from '@/api/endpoints/analytics';
import { useAnalyticsViewStore, type AnalyticsChartMetric } from './store';

interface TrendChartProps {
    query: UseQueryResult<AnalyticsResponseFormatted>;
}

export function TrendChart({ query }: TrendChartProps) {
    const t = useTranslations('analytics.chart');
    const tFeedback = useTranslations('analytics.feedback');
    const data = query.data;

    const chartMetric = useAnalyticsViewStore((state) => state.chartMetric);
    const setChartMetric = useAnalyticsViewStore((state) => state.setChartMetric);

    const getChartDataKey = (type: AnalyticsChartMetric) => {
        return type === 'cost' ? 'total_cost' : type === 'count' ? 'request_count' : 'total_token';
    };

    const chartData = useMemo(() => {
        if (!data) return [];
        const dataKey = getChartDataKey(chartMetric);
        return data.trend.map((point) => ({
            // 后端趋势日期为 YYYYMMDD，直接切片避免 Date 解析歧义
            date: `${point.date.slice(4, 6)}/${point.date.slice(6, 8)}`,
            [dataKey]: chartMetric === 'cost'
                ? point.metrics.total_cost.raw
                : chartMetric === 'count'
                    ? point.metrics.request_count.raw
                    : point.metrics.total_token.raw,
        }));
    }, [data, chartMetric]);

    const chartConfig = useMemo(() => {
        const labels = {
            cost: t('metricType.cost'),
            count: t('metricType.count'),
            tokens: t('metricType.tokens'),
        };
        return {
            [getChartDataKey(chartMetric)]: { label: labels[chartMetric] },
        };
    }, [chartMetric, t]);

    if (query.isLoading && !data) {
        return (
            <div className="flex h-64 items-center justify-center rounded-3xl border border-card-border bg-card text-muted-foreground custom-shadow">
                <Loader2 className="size-8 animate-spin" role="status" aria-label={tFeedback('loading')} />
            </div>
        );
    }

    if (query.isError && !data) {
        return (
            <div role="alert" className="flex h-64 flex-col items-center justify-center gap-3 rounded-3xl border border-destructive/30 bg-card p-4 text-center text-sm text-muted-foreground custom-shadow">
                <p>{tFeedback('loadFailed')}</p>
                <button type="button" onClick={() => void query.refetch()} className="rounded-xl border border-border px-3 py-1.5 font-medium text-foreground transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring">
                    {tFeedback('retry')}
                </button>
            </div>
        );
    }

    return (
        <div className="rounded-3xl bg-card border-card-border border pt-4 pb-0 text-card-foreground custom-shadow">
            {query.isError && data && (
                <div role="alert" className="mx-4 mb-2 flex items-center justify-between gap-3 rounded-2xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-muted-foreground">
                    <span>{tFeedback('loadFailed')}</span>
                    <button type="button" onClick={() => void query.refetch()} className="shrink-0 rounded-xl border border-border px-3 py-1.5 font-medium text-foreground transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring">
                        {tFeedback('retry')}
                    </button>
                </div>
            )}
            <div className="px-4 pb-2 flex justify-between items-center">
                <h3 className="font-semibold text-base">{t('title')}</h3>
                <Tabs value={chartMetric} onValueChange={(value) => setChartMetric(value as AnalyticsChartMetric)}>
                    <TabsList>
                        <TabsTrigger value="cost">{t('metricType.cost')}</TabsTrigger>
                        <TabsTrigger value="count">{t('metricType.count')}</TabsTrigger>
                        <TabsTrigger value="tokens">{t('metricType.tokens')}</TabsTrigger>
                    </TabsList>
                </Tabs>
            </div>
            {data && chartData.length === 0 ? (
                <div className="flex h-40 flex-col items-center justify-center gap-3 text-muted-foreground">
                    <TrendingUp className="w-12 h-12 opacity-30" />
                    <p className="text-sm">{t('empty')}</p>
                </div>
            ) : (
                <ChartContainer config={chartConfig} className="h-40 w-full">
                    <AreaChart accessibilityLayer data={chartData}>
                        <defs>
                            <linearGradient id="analyticsFillMetric1" x1="0" y1="0" x2="0" y2="1">
                                <stop offset="5%" stopColor="var(--chart-1)" stopOpacity={1.0} />
                                <stop offset="95%" stopColor="var(--chart-1)" stopOpacity={0.1} />
                            </linearGradient>
                            <linearGradient id="analyticsFillMetric2" x1="0" y1="0" x2="0" y2="1">
                                <stop offset="5%" stopColor="var(--chart-2)" stopOpacity={1.0} />
                                <stop offset="95%" stopColor="var(--chart-2)" stopOpacity={0.1} />
                            </linearGradient>
                            <linearGradient id="analyticsFillMetric3" x1="0" y1="0" x2="0" y2="1">
                                <stop offset="5%" stopColor="var(--chart-3)" stopOpacity={1.0} />
                                <stop offset="95%" stopColor="var(--chart-3)" stopOpacity={0.1} />
                            </linearGradient>
                        </defs>
                        <CartesianGrid strokeDasharray="3 3" vertical={false} />
                        <XAxis dataKey="date" tickLine={false} axisLine={false} />
                        <YAxis
                            tickLine={false}
                            axisLine={false}
                            tickFormatter={(value) => {
                                if (chartMetric === 'cost') {
                                    const formatted = formatMoney(value);
                                    return `${formatted.formatted.value}${formatted.formatted.unit}`;
                                }
                                const formatted = formatCount(value);
                                return `${formatted.formatted.value}${formatted.formatted.unit}`;
                            }}
                        />
                        <ChartTooltip cursor={false} content={<ChartTooltipContent indicator="line" />} />
                        <Area
                            type="monotone"
                            dataKey={getChartDataKey(chartMetric)}
                            stroke={chartMetric === 'cost' ? 'var(--chart-1)' : chartMetric === 'count' ? 'var(--chart-2)' : 'var(--chart-3)'}
                            fill={chartMetric === 'cost' ? 'url(#analyticsFillMetric1)' : chartMetric === 'count' ? 'url(#analyticsFillMetric2)' : 'url(#analyticsFillMetric3)'}
                        />
                    </AreaChart>
                </ChartContainer>
            )}
        </div>
    );
}
