import { useMemo } from 'react';
import { usePrefersReducedMotion } from '@/hooks/use-prefers-reduced-motion';
import { DURATION } from '@/lib/animations/fluid-transitions';
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts';
import { useTranslations } from 'use-intl';
import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart';
import { formatCount, formatMoney, formatTime } from '@/lib/utils';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Loader2, TrendingUp } from 'lucide-react';
import type { UseQueryResult } from '@tanstack/react-query';
import type { AnalyticsMetricsFormatted, AnalyticsResponseFormatted } from '@/api/endpoints/analytics';
import { useAnalyticsViewStore, type AnalyticsChartMetric } from './store';
import { InlineRetry } from '@/components/common/InlineRetry';

interface TrendChartProps {
    query: UseQueryResult<AnalyticsResponseFormatted>;
}

const METRIC_KEYS = {
    cost: 'total_cost', count: 'request_count', tokens: 'total_token', input: 'input_token',
    output: 'output_token', cached: 'observed_cached_token', cacheRatio: 'cache_read_ratio',
    success: 'success_rate', failed: 'request_failed', latency: 'average_wait_time',
} as const satisfies Record<AnalyticsChartMetric, keyof AnalyticsMetricsFormatted>;

export function TrendChart({ query }: TrendChartProps) {
    const t = useTranslations('analytics.chart');
    const tFeedback = useTranslations('analytics.feedback');
    const shouldReduceMotion = usePrefersReducedMotion();
    const data = query.data;
    const chartMetric = useAnalyticsViewStore((state) => state.chartMetric);
    const setChartMetric = useAnalyticsViewStore((state) => state.setChartMetric);
    const chartData = useMemo(() => data?.trend.map((point) => ({
        date: `${point.date.slice(4, 6)}/${point.date.slice(6, 8)}`,
        value: point.metrics[METRIC_KEYS[chartMetric]]?.raw ?? null,
    })) ?? [], [data, chartMetric]);
    const config = { value: { label: t(`metricType.${chartMetric}`) } };
    const formatValue = (value: number) => {
        if (chartMetric === 'cacheRatio' || chartMetric === 'success') return `${value.toFixed(1)}%`;
        // 极小成本用有效数字，避免纵轴出现一排相同的 0.00$
        if (chartMetric === 'cost' && value > 0 && value < 0.01) return `${value.toPrecision(2)}$`;
        const formatted = chartMetric === 'cost' ? formatMoney(value) : chartMetric === 'latency' ? formatTime(value) : formatCount(value);
        return `${formatted.formatted.value}${formatted.formatted.unit}`;
    };

    if (query.isLoading && !data) {
        return <div className="flex h-64 items-center justify-center rounded-3xl border border-border bg-card"><Loader2 className="size-8 animate-spin" role="status" aria-label={tFeedback('loading')} /></div>;
    }
    if (query.isError && !data) {
        return (
            <div role="alert" className="flex h-64 flex-col items-center justify-center gap-3 rounded-3xl border border-destructive/30 bg-card p-4 text-center text-sm text-muted-foreground">
                <p>{tFeedback('loadFailed')}</p>
                <InlineRetry onRetry={() => void query.refetch()} label={tFeedback('retry')} />
            </div>
        );
    }
    return (
        <section className="rounded-3xl border border-border bg-card pt-4 pb-2 text-card-foreground">
            {query.isError && data && <p role="alert" className="px-4 pb-2 text-sm text-destructive">{tFeedback('loadFailed')}</p>}
            <div className="flex flex-wrap items-center justify-between gap-3 px-4 pb-3">
                <h3 className="text-base font-semibold">{t('title')}</h3>
                <Select value={chartMetric} onValueChange={(value) => setChartMetric(value as AnalyticsChartMetric)}>
                    <SelectTrigger aria-label={t('metricLabel')} className="w-44"><SelectValue /></SelectTrigger>
                    <SelectContent>
                        {(Object.keys(METRIC_KEYS) as AnalyticsChartMetric[]).map((metric) => <SelectItem key={metric} value={metric}>{t(`metricType.${metric}`)}</SelectItem>)}
                    </SelectContent>
                </Select>
            </div>
            {chartData.length === 0 ? (
                <div className="flex h-40 flex-col items-center justify-center gap-3 text-muted-foreground"><TrendingUp className="size-10 opacity-40" aria-hidden="true" /><p className="text-sm">{t('empty')}</p></div>
            ) : (
                <ChartContainer config={config} className="h-48 w-full">
                    <AreaChart accessibilityLayer data={chartData} margin={{ left: 4, right: 16 }}>
                        <defs><linearGradient id="analyticsTrendFill" x1="0" y1="0" x2="0" y2="1"><stop offset="5%" stopColor="var(--chart-1)" stopOpacity={0.6} /><stop offset="95%" stopColor="var(--chart-1)" stopOpacity={0.03} /></linearGradient></defs>
                        <CartesianGrid strokeDasharray="3 3" vertical={false} />
                        <XAxis dataKey="date" tickLine={false} axisLine={false} minTickGap={24} />
                        <YAxis tickLine={false} axisLine={false} tickFormatter={formatValue} width={72} />
                        <ChartTooltip cursor={false} content={<ChartTooltipContent indicator="line" formatter={(value) => <span className="tabular-nums">{formatValue(Number(value))}</span>} />} />
                        <Area type="monotone" dataKey="value" stroke="var(--chart-1)" fill="url(#analyticsTrendFill)" strokeWidth={2} isAnimationActive={!shouldReduceMotion} animationDuration={DURATION.panel * 1000} animationEasing="ease-in-out" />
                    </AreaChart>
                </ChartContainer>
            )}
        </section>
    );
}
