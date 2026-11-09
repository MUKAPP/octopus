import { useMemo, useState } from 'react';
import { useLocale, useTranslations } from 'use-intl';
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from 'recharts';
import { ChartNoAxesColumn, Gauge, RotateCcw, Timer, Waves } from 'lucide-react';
import type { UseQueryResult } from '@tanstack/react-query';
import type { AnalyticsDistribution, AnalyticsFeatureRow, AnalyticsResponseFormatted, FormattedValue } from '@/api/endpoints/analytics';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart';
import { formatCount, formatMoney, formatTime } from '@/lib/utils';
import { cn } from '@/lib/utils';

interface Props { query: UseQueryResult<AnalyticsResponseFormatted> }
type View = 'efficiency' | 'performance' | 'reliability' | 'traffic' | 'features';
type Feature = 'stream' | 'reasoning_effort' | 'client_protocol' | 'task_type';
const VIEWS: View[] = ['efficiency', 'performance', 'reliability', 'traffic', 'features'];
const FEATURES: Feature[] = ['stream', 'reasoning_effort', 'client_protocol', 'task_type'];
const VIEW_ICONS: Record<View, typeof Gauge> = {
    efficiency: ChartNoAxesColumn,
    performance: Timer,
    reliability: RotateCcw,
    traffic: Waves,
    features: Gauge,
};
const display = (value: FormattedValue) => value.formatted.value + value.formatted.unit;
const count = (value: number | null) => value === null ? '—' : display(formatCount(value));
const money = (value: number | null) => value === null ? '—' : value > 0 && value < 0.01 ? value.toPrecision(3) + '$' : display(formatMoney(value));
const duration = (value: number | null) => value === null ? '—' : display(formatTime(value));
const ratio = (part: number, total: number) => total > 0 ? (part / total * 100).toFixed(1) + '%' : '—';

function Metric({ label, value, hint, emphasis }: { label: string; value: string; hint?: string; emphasis?: boolean }) {
    return (
        <div className="min-w-0 rounded-2xl bg-muted/25 px-3 py-3">
            <dt className="truncate text-xs text-muted-foreground">{label}</dt>
            <dd className={cn('mt-1 break-words font-medium tabular-nums', emphasis ? 'text-2xl' : 'text-lg')}>{value}</dd>
            {hint && <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{hint}</p>}
        </div>
    );
}

export function AnalysisDetails({ query }: Props) {
    const t = useTranslations('analytics.detail');
    const tFilter = useTranslations('analytics.filter');
    const locale = useLocale();
    const [view, setView] = useState<View>('efficiency');
    const [feature, setFeature] = useState<Feature>('stream');
    const [trafficMetric, setTrafficMetric] = useState<'requests' | 'cost'>('requests');
    const data = query.data;
    const details = data?.details;
    const timeLabel = (value: string | number | null) => value === null || value === '' ? '—' : new Date(value).toLocaleString(locale, { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
    const hourly = useMemo(() => details?.hourly.map((point) => ({
        hour: point.hour, requests: point.request_count, cost: point.input_cost + point.output_cost,
    })) ?? [], [details]);
    const hourLabels = useMemo(() => {
        const buckets = Array.from({ length: 24 }, (_, hour) => ({ hour, requests: 0 }));
        for (const point of details?.hourly ?? []) {
            // RFC3339 包含服务器时区，直接读取日历小时，避免被浏览器时区改写。
            const hour = Number(point.hour.slice(11, 13));
            buckets[hour].requests += point.request_count;
        }
        return buckets;
    }, [details]);
    if (!data || !details) return null;
    const summary = data.summary;
    const coverage = details.coverage;
    const samples = (value: number) => t('samples', { count: value });
    const distribution = (label: string, value: AnalyticsDistribution) => (
        <div className="min-w-0 rounded-2xl border border-border/60 bg-muted/15 p-4">
            <h4 className="text-sm font-medium">{label}</h4>
            <dl className="mt-2 grid grid-cols-2 gap-3">
                <Metric label={t('p50')} value={duration(value.p50)} />
                <Metric label={t('p95')} value={duration(value.p95)} />
            </dl>
            <p className="mt-2 text-xs text-muted-foreground">{samples(value.sample_count)}</p>
        </div>
    );
    const featureRows: AnalyticsFeatureRow[] = details[
        feature === 'stream' ? 'by_stream' : feature === 'reasoning_effort' ? 'by_reasoning_effort' : feature === 'client_protocol' ? 'by_client_protocol' : 'by_task_type'
    ];
    const featureName = (name: string) => {
        if (feature === 'stream') return t(name === 'true' ? 'streaming' : 'nonStreaming');
        if (feature === 'task_type' && t.has('task.' + name)) return t('task.' + name);
        return name || tFilter('unknown');
    };
    const peakHour = Math.max(...hourLabels.map((point) => point.requests));

    return (
        <section className="rounded-3xl border border-card-border bg-card p-3 text-card-foreground sm:p-4">
            <div className="flex flex-col gap-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                    <h3 className="text-base font-semibold">{t('title')}</h3>
                    <span className="max-w-full rounded-full bg-muted/30 px-2.5 py-1 text-xs text-muted-foreground sm:max-w-md sm:truncate">
                        {coverage.available_from_ms === null ? t('noCollection') : t('coverage', { date: timeLabel(coverage.available_from_ms), count: coverage.sample_count })}
                    </span>
                </div>
                <div role="group" aria-label={t('viewLabel')} className="grid grid-cols-3 gap-1 sm:flex sm:flex-wrap">
                    {VIEWS.map((item) => {
                        const Icon = VIEW_ICONS[item];
                        return (
                            <button
                                key={item}
                                type="button"
                                aria-pressed={view === item}
                                onClick={() => setView(item)}
                                className={cn(
                                    'flex min-w-0 items-center justify-center gap-1.5 rounded-xl px-2 py-1.5 text-xs transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring sm:shrink-0 sm:px-2.5 sm:text-sm',
                                    view === item
                                        ? 'bg-primary/10 font-medium text-foreground ring-1 ring-primary/25'
                                        : 'text-muted-foreground hover:bg-muted/30 hover:text-foreground'
                                )}
                            >
                                <Icon className="size-4" />
                                {t('view.' + item)}
                            </button>
                        );
                    })}
                </div>
                {(coverage.legacy_request_count > 0 || coverage.summary_source === 'request_metadata' || query.isError) && (
                    <div role="note" className="space-y-1 rounded-2xl bg-muted/25 px-3 py-2 text-xs leading-relaxed text-muted-foreground">
                        {coverage.legacy_request_count > 0 && <p>{t('legacy', { count: coverage.legacy_request_count })}</p>}
                        {coverage.summary_source === 'request_metadata' && <p className="font-medium text-foreground">{t('metadataSource')}</p>}
                    </div>
                )}
            </div>
            {query.isError && <p role="alert" className="mt-3 text-sm text-destructive">{t('stale')}</p>}

            {view === 'efficiency' && <>
                <dl className="mt-3 grid grid-cols-2 gap-2 md:grid-cols-4">
                    <Metric label={t('inputTokens')} value={display(summary.input_token)} />
                    <Metric label={t('outputTokens')} value={display(summary.output_token)} />
                    <Metric label={t('cacheRead')} value={summary.observed_cached_token === null ? '—' : display(summary.observed_cached_token)} hint={t('cacheRatio') + ': ' + summary.cache_read_ratio.formatted + ' · ' + samples(summary.cache_ratio_sample_count)} />
                    <Metric label={t('averageCost')} value={summary.request_count.raw > 0 ? display(summary.average_cost) : '—'} />
                    <Metric label={t('inputCost')} value={display(summary.input_cost)} hint={t('costShare') + ': ' + ratio(summary.input_cost.raw, summary.total_cost.raw)} />
                    <Metric label={t('outputCost')} value={display(summary.output_cost)} hint={t('costShare') + ': ' + ratio(summary.output_cost.raw, summary.total_cost.raw)} />
                    <Metric label={t('cacheWrite')} value={count(details.tokens.cache_write_token)} hint={samples(details.tokens.cache_write_sample_count)} />
                    <Metric label={t('reasoningTokens')} value={count(details.tokens.reasoning_token)} hint={samples(details.tokens.reasoning_sample_count)} />
                </dl>
                <p className="mt-3 text-xs leading-relaxed text-muted-foreground">{t('cacheNote')}</p>
            </>}

            {view === 'performance' && <>
                <dl className="mt-3 grid grid-cols-2 gap-2 md:grid-cols-4">
                    <Metric label={t('averageDuration')} value={summary.request_count.raw > 0 ? display(summary.average_wait_time) : '—'} />
                    <Metric label={t('outputSpeed')} value={details.performance.stream_output_tokens_per_second.value === null ? '—' : t('tokensPerSecond', { value: details.performance.stream_output_tokens_per_second.value.toFixed(1) })} hint={samples(details.performance.stream_output_tokens_per_second.sample_count)} />
                    <Metric label={t('speedP50')} value={details.performance.stream_output_tokens_per_second.p50 === null ? '—' : t('tokensPerSecond', { value: details.performance.stream_output_tokens_per_second.p50.toFixed(1) })} />
                    <Metric label={t('speedP95')} value={details.performance.stream_output_tokens_per_second.p95 === null ? '—' : t('tokensPerSecond', { value: details.performance.stream_output_tokens_per_second.p95.toFixed(1) })} />
                </dl>
                <div className="mt-3 grid gap-3 md:grid-cols-2">{distribution(t('firstToken'), details.performance.ttft_ms)}{distribution(t('totalDuration'), details.performance.total_duration_ms)}</div>
                <p className="mt-3 text-xs leading-relaxed text-muted-foreground">{t('performanceNote')}</p>
            </>}

            {view === 'reliability' && <>
                <dl className="mt-3 grid grid-cols-2 gap-2 md:grid-cols-4">
                    <Metric label={t('failedRequests')} value={display(summary.request_failed)} />
                    <Metric label={t('retryRequests')} value={coverage.sample_count > 0 ? count(details.reliability.retry_requests) : '—'} hint={t('retryRate') + ': ' + ratio(details.reliability.retry_requests, coverage.sample_count)} />
                    <Metric label={t('recoveredRequests')} value={coverage.sample_count > 0 ? count(details.reliability.recovered_requests) : '—'} hint={t('recoveryRate') + ': ' + ratio(details.reliability.recovered_requests, details.reliability.retry_requests)} />
                    <Metric label={t('executedAttempts')} value={coverage.sample_count > 0 ? count(details.reliability.executed_attempts) : '—'} />
                    <Metric label={t('retryAttempts')} value={coverage.sample_count > 0 ? count(details.reliability.retry_attempts) : '—'} />
                    <Metric label={t('circuitSkips')} value={coverage.sample_count > 0 ? count(details.reliability.circuit_break_skips) : '—'} />
                    <Metric label={t('otherSkips')} value={coverage.sample_count > 0 ? count(details.reliability.other_skips) : '—'} />
                </dl>
                <div className="mt-3 grid gap-3 md:grid-cols-2">
                    <div className="rounded-2xl border border-border/60 p-3"><h4 className="mb-2 text-sm font-medium">{t('errorCategory')}</h4><Table><TableHeader><TableRow><TableHead>{t('category')}</TableHead><TableHead className="text-right">{t('requests')}</TableHead></TableRow></TableHeader><TableBody>
                        {details.by_error_category.map((item) => <TableRow key={item.name}><TableCell>{t.has('error.' + item.name) ? t('error.' + item.name) : item.name || tFilter('unknown')}</TableCell><TableCell className="text-right tabular-nums">{count(item.count)}</TableCell></TableRow>)}
                        {details.by_error_category.length === 0 && <TableRow><TableCell colSpan={2} className="text-muted-foreground">{t('noSamples')}</TableCell></TableRow>}
                    </TableBody></Table></div>
                    <div className="rounded-2xl border border-border/60 p-3"><h4 className="mb-2 text-sm font-medium">{t('statusCodes')}</h4><Table><TableHeader><TableRow><TableHead>{t('statusCode')}</TableHead><TableHead className="text-right">{t('requests')}</TableHead></TableRow></TableHeader><TableBody>
                        {details.by_status_code.map((item) => <TableRow key={item.status_code}><TableCell>{item.status_code || t('noUpstreamResponse')}</TableCell><TableCell className="text-right tabular-nums">{count(item.count)}</TableCell></TableRow>)}
                        {details.by_status_code.length === 0 && <TableRow><TableCell colSpan={2} className="text-muted-foreground">{t('noSamples')}</TableCell></TableRow>}
                    </TableBody></Table></div>
                </div>
                <p className="mt-3 text-xs leading-relaxed text-muted-foreground">{t('retryNote')}</p>
            </>}

            {view === 'traffic' && <>
                <dl className="mt-3 grid grid-cols-2 gap-2"><Metric label={t('peakRpm')} value={count(details.peaks.rpm)} hint={timeLabel(details.peaks.rpm_at)} /><Metric label={t('peakTpm')} value={count(details.peaks.tpm)} hint={timeLabel(details.peaks.tpm_at)} /></dl>
                <div className="mt-4 flex flex-wrap items-center justify-between gap-3"><h4 className="text-sm font-medium">{t('hourly')}</h4><Select value={trafficMetric} onValueChange={(value) => setTrafficMetric(value as typeof trafficMetric)}><SelectTrigger aria-label={t('trafficMetric')} size="sm" className="w-32 rounded-xl"><SelectValue /></SelectTrigger><SelectContent className="rounded-xl"><SelectItem value="requests">{t('requests')}</SelectItem><SelectItem value="cost">{t('cost')}</SelectItem></SelectContent></Select></div>
                {hourly.length === 0 ? <p className="py-8 text-center text-sm text-muted-foreground">{t('noSamples')}</p> : <ChartContainer config={{ requests: { label: t('requests') }, cost: { label: t('cost') } }} className="mt-3 h-56 w-full">
                    <BarChart accessibilityLayer data={hourly} margin={{ right: 12 }}><CartesianGrid vertical={false} strokeDasharray="3 3" /><XAxis dataKey="hour" tickLine={false} axisLine={false} minTickGap={36} tickFormatter={(value: string) => value.slice(5, 16).replace('T', ' ')} /><YAxis width={64} tickLine={false} axisLine={false} tickFormatter={(value: number) => trafficMetric === 'cost' ? money(value) : count(value)} /><ChartTooltip content={<ChartTooltipContent labelFormatter={(value) => timeLabel(String(value))} />} /><Bar dataKey={trafficMetric} fill="var(--chart-1)" radius={[4, 4, 0, 0]} maxBarSize={56} isAnimationActive={false} /></BarChart>
                </ChartContainer>}
                <h4 className="mt-4 text-sm font-medium">{t('hourHeatmap')}</h4>
                <div className="mt-2 grid grid-cols-6 gap-1.5 sm:grid-cols-8 lg:grid-cols-12">{hourLabels.map((point) => (
                    <div
                        key={point.hour}
                        title={t('hourValue', { hour: point.hour, count: point.requests })}
                        className="rounded-lg border border-border/40 px-1 py-1.5 text-center text-[11px] tabular-nums"
                        style={{ backgroundColor: peakHour > 0 && point.requests > 0 ? `color-mix(in oklch, var(--primary) ${Math.round(point.requests / peakHour * 60) + 8}%, var(--card))` : undefined }}
                    >
                        <span className="text-muted-foreground">{String(point.hour).padStart(2, '0')}</span>
                        <span className="mt-0.5 block font-medium">{coverage.sample_count > 0 ? point.requests : '—'}</span>
                    </div>
                ))}</div>
                <p className="mt-3 text-xs leading-relaxed text-muted-foreground">{t('trafficNote')}</p>
            </>}

            {view === 'features' && <>
                <div className="mt-3"><Select value={feature} onValueChange={(value) => setFeature(value as Feature)}><SelectTrigger aria-label={t('featureLabel')} size="sm" className="w-44 rounded-xl"><SelectValue /></SelectTrigger><SelectContent className="rounded-xl">{FEATURES.map((item) => <SelectItem key={item} value={item}>{t('feature.' + item)}</SelectItem>)}</SelectContent></Select></div>
                <div className="mt-3 overflow-x-auto"><Table><TableHeader><TableRow><TableHead>{t('category')}</TableHead><TableHead className="text-right">{t('requests')}</TableHead><TableHead className="text-right">{t('successRate')}</TableHead><TableHead className="text-right">{t('inputTokens')}</TableHead><TableHead className="text-right">{t('outputTokens')}</TableHead><TableHead className="text-right">{t('cost')}</TableHead><TableHead className="text-right">{t('averageCost')}</TableHead><TableHead className="text-right">{t('durationP95')}</TableHead><TableHead className="text-right">{t('ttftP50')}</TableHead><TableHead className="text-right">{t('outputSpeed')}</TableHead></TableRow></TableHeader><TableBody>
                    {featureRows.map((row) => <TableRow key={row.name}><TableCell className="font-medium">{featureName(row.name)}</TableCell><TableCell className="text-right tabular-nums">{count(row.sample_count)}</TableCell><TableCell className="text-right tabular-nums">{ratio(row.request_success, row.sample_count)}</TableCell><TableCell className="text-right tabular-nums">{count(row.input_token)}</TableCell><TableCell className="text-right tabular-nums">{count(row.output_token)}</TableCell><TableCell className="text-right tabular-nums">{money(row.input_cost + row.output_cost)}</TableCell><TableCell className="text-right tabular-nums">{row.sample_count > 0 ? money((row.input_cost + row.output_cost) / row.sample_count) : '—'}</TableCell><TableCell className="text-right tabular-nums">{duration(row.total_duration_ms.p95)}</TableCell><TableCell className="text-right tabular-nums">{duration(row.ttft_ms.p50)}</TableCell><TableCell className="text-right tabular-nums">{row.stream_output_tokens_per_second.value === null ? '—' : t('tokensPerSecond', { value: row.stream_output_tokens_per_second.value.toFixed(1) })}</TableCell></TableRow>)}
                    {featureRows.length === 0 && <TableRow><TableCell colSpan={10} className="text-muted-foreground">{t('noSamples')}</TableCell></TableRow>}
                </TableBody></Table></div>
            </>}
        </section>
    );
}
