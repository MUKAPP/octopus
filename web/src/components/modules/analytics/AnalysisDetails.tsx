import { useMemo, useState } from 'react';
import { useLocale, useTranslations } from 'use-intl';
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from 'recharts';
import type { UseQueryResult } from '@tanstack/react-query';
import type { AnalyticsDistribution, AnalyticsFeatureRow, AnalyticsResponseFormatted, FormattedValue } from '@/api/endpoints/analytics';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { ChartContainer, ChartTooltip, ChartTooltipContent } from '@/components/ui/chart';
import { formatCount, formatMoney, formatTime } from '@/lib/utils';

interface Props { query: UseQueryResult<AnalyticsResponseFormatted> }
type View = 'efficiency' | 'performance' | 'reliability' | 'traffic' | 'features';
type Feature = 'stream' | 'reasoning_effort' | 'client_protocol' | 'task_type';
const VIEWS: View[] = ['efficiency', 'performance', 'reliability', 'traffic', 'features'];
const FEATURES: Feature[] = ['stream', 'reasoning_effort', 'client_protocol', 'task_type'];
const display = (value: FormattedValue) => value.formatted.value + value.formatted.unit;
const count = (value: number | null) => value === null ? '—' : display(formatCount(value));
const money = (value: number | null) => value === null ? '—' : value > 0 && value < 0.01 ? value.toPrecision(3) + '$' : display(formatMoney(value));
const duration = (value: number | null) => value === null ? '—' : display(formatTime(value));
const ratio = (part: number, total: number) => total > 0 ? (part / total * 100).toFixed(1) + '%' : '—';

function Metric({ label, value, hint }: { label: string; value: string; hint?: string }) {
    return <div className="min-w-0 border-b border-border/50 py-3"><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1 break-words text-xl font-medium tabular-nums">{value}</dd>{hint && <p className="mt-1 text-xs text-muted-foreground">{hint}</p>}</div>;
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
        <div className="min-w-0 rounded-2xl border border-border/60 p-4">
            <h4 className="text-sm font-medium">{label}</h4>
            <dl className="mt-2 grid grid-cols-2 gap-4"><Metric label={t('p50')} value={duration(value.p50)} /><Metric label={t('p95')} value={duration(value.p95)} /></dl>
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
        <section className="rounded-3xl border border-card-border bg-card p-4 text-card-foreground">
            <div className="flex flex-wrap items-center justify-between gap-3">
                <h3 className="text-base font-semibold">{t('title')}</h3>
                <Select value={view} onValueChange={(value) => setView(value as View)}>
                    <SelectTrigger aria-label={t('viewLabel')} className="w-48 rounded-xl"><SelectValue /></SelectTrigger>
                    <SelectContent className="rounded-xl">{VIEWS.map((item) => <SelectItem key={item} value={item}>{t('view.' + item)}</SelectItem>)}</SelectContent>
                </Select>
            </div>
            <div role="note" className="mt-3 space-y-1 rounded-2xl bg-muted/30 p-3 text-xs leading-relaxed text-muted-foreground">
                <p>{coverage.available_from_ms === null ? t('noCollection') : t('coverage', { date: timeLabel(coverage.available_from_ms), count: coverage.sample_count })}</p>
                {coverage.legacy_request_count > 0 && <p>{t('legacy', { count: coverage.legacy_request_count })}</p>}
                {coverage.summary_source === 'request_metadata' && <p className="font-medium text-foreground">{t('metadataSource')}</p>}
            </div>
            {query.isError && <p role="alert" className="mt-3 text-sm text-destructive">{t('stale')}</p>}

            {view === 'efficiency' && <>
                <dl className="mt-3 grid grid-cols-2 gap-x-6 md:grid-cols-4">
                    <Metric label={t('inputTokens')} value={display(summary.input_token)} />
                    <Metric label={t('outputTokens')} value={display(summary.output_token)} />
                    <Metric label={t('cacheRead')} value={summary.observed_cached_token === null ? '—' : display(summary.observed_cached_token)} hint={t('cacheRatio') + ': ' + summary.cache_read_ratio.formatted + ' · ' + samples(summary.cache_read_sample_count)} />
                    <Metric label={t('averageCost')} value={summary.request_count.raw > 0 ? display(summary.average_cost) : '—'} />
                    <Metric label={t('inputCost')} value={display(summary.input_cost)} hint={t('costShare') + ': ' + ratio(summary.input_cost.raw, summary.total_cost.raw)} />
                    <Metric label={t('outputCost')} value={display(summary.output_cost)} hint={t('costShare') + ': ' + ratio(summary.output_cost.raw, summary.total_cost.raw)} />
                    <Metric label={t('cacheWrite')} value={count(details.tokens.cache_write_token)} hint={samples(details.tokens.cache_write_sample_count)} />
                    <Metric label={t('reasoningTokens')} value={count(details.tokens.reasoning_token)} hint={samples(details.tokens.reasoning_sample_count)} />
                </dl>
                <p className="mt-4 text-xs leading-relaxed text-muted-foreground">{t('cacheNote')}</p>
            </>}

            {view === 'performance' && <>
                <dl className="mt-3 grid grid-cols-2 gap-x-6 md:grid-cols-4">
                    <Metric label={t('averageDuration')} value={summary.request_count.raw > 0 ? display(summary.average_wait_time) : '—'} />
                    <Metric label={t('outputSpeed')} value={details.performance.stream_output_tokens_per_second.value === null ? '—' : t('tokensPerSecond', { value: details.performance.stream_output_tokens_per_second.value.toFixed(1) })} hint={samples(details.performance.stream_output_tokens_per_second.sample_count)} />
                    <Metric label={t('speedP50')} value={details.performance.stream_output_tokens_per_second.p50 === null ? '—' : t('tokensPerSecond', { value: details.performance.stream_output_tokens_per_second.p50.toFixed(1) })} />
                    <Metric label={t('speedP95')} value={details.performance.stream_output_tokens_per_second.p95 === null ? '—' : t('tokensPerSecond', { value: details.performance.stream_output_tokens_per_second.p95.toFixed(1) })} />
                </dl>
                <div className="mt-4 grid gap-4 md:grid-cols-2">{distribution(t('firstToken'), details.performance.ttft_ms)}{distribution(t('totalDuration'), details.performance.total_duration_ms)}</div>
                <p className="mt-4 text-xs leading-relaxed text-muted-foreground">{t('performanceNote')}</p>
            </>}

            {view === 'reliability' && <>
                <dl className="mt-3 grid grid-cols-2 gap-x-6 md:grid-cols-4">
                    <Metric label={t('failedRequests')} value={display(summary.request_failed)} />
                    <Metric label={t('retryRequests')} value={coverage.sample_count > 0 ? count(details.reliability.retry_requests) : '—'} hint={t('retryRate') + ': ' + ratio(details.reliability.retry_requests, coverage.sample_count)} />
                    <Metric label={t('recoveredRequests')} value={coverage.sample_count > 0 ? count(details.reliability.recovered_requests) : '—'} hint={t('recoveryRate') + ': ' + ratio(details.reliability.recovered_requests, details.reliability.retry_requests)} />
                    <Metric label={t('executedAttempts')} value={coverage.sample_count > 0 ? count(details.reliability.executed_attempts) : '—'} />
                    <Metric label={t('retryAttempts')} value={coverage.sample_count > 0 ? count(details.reliability.retry_attempts) : '—'} />
                    <Metric label={t('circuitSkips')} value={coverage.sample_count > 0 ? count(details.reliability.circuit_break_skips) : '—'} />
                    <Metric label={t('otherSkips')} value={coverage.sample_count > 0 ? count(details.reliability.other_skips) : '—'} />
                </dl>
                <div className="mt-4 grid gap-4 md:grid-cols-2">
                    <div><h4 className="mb-2 text-sm font-medium">{t('errorCategory')}</h4><Table><TableHeader><TableRow><TableHead>{t('category')}</TableHead><TableHead className="text-right">{t('requests')}</TableHead></TableRow></TableHeader><TableBody>
                        {details.by_error_category.map((item) => <TableRow key={item.name}><TableCell>{t.has('error.' + item.name) ? t('error.' + item.name) : item.name || tFilter('unknown')}</TableCell><TableCell className="text-right tabular-nums">{count(item.count)}</TableCell></TableRow>)}
                        {details.by_error_category.length === 0 && <TableRow><TableCell colSpan={2}>{t('noSamples')}</TableCell></TableRow>}
                    </TableBody></Table></div>
                    <div><h4 className="mb-2 text-sm font-medium">{t('statusCodes')}</h4><Table><TableHeader><TableRow><TableHead>{t('statusCode')}</TableHead><TableHead className="text-right">{t('requests')}</TableHead></TableRow></TableHeader><TableBody>
                        {details.by_status_code.map((item) => <TableRow key={item.status_code}><TableCell>{item.status_code || t('noUpstreamResponse')}</TableCell><TableCell className="text-right tabular-nums">{count(item.count)}</TableCell></TableRow>)}
                        {details.by_status_code.length === 0 && <TableRow><TableCell colSpan={2}>{t('noSamples')}</TableCell></TableRow>}
                    </TableBody></Table></div>
                </div>
                <p className="mt-4 text-xs leading-relaxed text-muted-foreground">{t('retryNote')}</p>
            </>}

            {view === 'traffic' && <>
                <dl className="mt-3 grid grid-cols-2 gap-x-6"><Metric label={t('peakRpm')} value={count(details.peaks.rpm)} hint={timeLabel(details.peaks.rpm_at)} /><Metric label={t('peakTpm')} value={count(details.peaks.tpm)} hint={timeLabel(details.peaks.tpm_at)} /></dl>
                <div className="mt-4 flex flex-wrap items-center justify-between gap-3"><h4 className="text-sm font-medium">{t('hourly')}</h4><Select value={trafficMetric} onValueChange={(value) => setTrafficMetric(value as typeof trafficMetric)}><SelectTrigger aria-label={t('trafficMetric')} className="w-36 rounded-xl"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="requests">{t('requests')}</SelectItem><SelectItem value="cost">{t('cost')}</SelectItem></SelectContent></Select></div>
                {hourly.length === 0 ? <p className="py-8 text-center text-sm text-muted-foreground">{t('noSamples')}</p> : <ChartContainer config={{ requests: { label: t('requests') }, cost: { label: t('cost') } }} className="mt-3 h-56 w-full">
                    <BarChart accessibilityLayer data={hourly} margin={{ right: 12 }}><CartesianGrid vertical={false} /><XAxis dataKey="hour" tickLine={false} axisLine={false} minTickGap={36} tickFormatter={(value: string) => value.slice(5, 16).replace('T', ' ')} /><YAxis width={64} tickFormatter={(value: number) => trafficMetric === 'cost' ? money(value) : count(value)} /><ChartTooltip content={<ChartTooltipContent labelFormatter={(value) => timeLabel(String(value))} />} /><Bar dataKey={trafficMetric} fill="var(--chart-1)" radius={[3, 3, 0, 0]} isAnimationActive={false} /></BarChart>
                </ChartContainer>}
                <h4 className="mt-4 text-sm font-medium">{t('hourHeatmap')}</h4>
                <div className="scrollbar mt-3 overflow-x-auto pb-2"><div className="grid min-w-[36rem] grid-cols-12 gap-1.5">{hourLabels.map((point) => <div key={point.hour} title={t('hourValue', { hour: point.hour, count: point.requests })} className="rounded-lg border border-border/50 p-2 text-center text-xs tabular-nums" style={{ backgroundColor: peakHour > 0 && point.requests > 0 ? `color-mix(in oklch, var(--primary) ${Math.round(point.requests / peakHour * 65) + 5}%, var(--card))` : undefined }}><span>{String(point.hour).padStart(2, '0')}</span><span className="mt-1 block font-medium">{coverage.sample_count > 0 ? count(point.requests) : '—'}</span></div>)}</div></div>
                <p className="mt-3 text-xs leading-relaxed text-muted-foreground">{t('trafficNote')}</p>
            </>}

            {view === 'features' && <>
                <div className="mt-4"><Select value={feature} onValueChange={(value) => setFeature(value as Feature)}><SelectTrigger aria-label={t('featureLabel')} className="w-48 rounded-xl"><SelectValue /></SelectTrigger><SelectContent>{FEATURES.map((item) => <SelectItem key={item} value={item}>{t('feature.' + item)}</SelectItem>)}</SelectContent></Select></div>
                <Table className="mt-3"><TableHeader><TableRow><TableHead>{t('category')}</TableHead><TableHead>{t('requests')}</TableHead><TableHead>{t('successRate')}</TableHead><TableHead>{t('inputTokens')}</TableHead><TableHead>{t('outputTokens')}</TableHead><TableHead>{t('cost')}</TableHead><TableHead>{t('averageCost')}</TableHead><TableHead>{t('durationP95')}</TableHead><TableHead>{t('ttftP50')}</TableHead><TableHead>{t('outputSpeed')}</TableHead></TableRow></TableHeader><TableBody>
                    {featureRows.map((row) => <TableRow key={row.name}><TableCell className="font-medium">{featureName(row.name)}</TableCell><TableCell>{count(row.sample_count)}</TableCell><TableCell>{ratio(row.request_success, row.sample_count)}</TableCell><TableCell>{count(row.input_token)}</TableCell><TableCell>{count(row.output_token)}</TableCell><TableCell>{money(row.input_cost + row.output_cost)}</TableCell><TableCell>{row.sample_count > 0 ? money((row.input_cost + row.output_cost) / row.sample_count) : '—'}</TableCell><TableCell>{duration(row.total_duration_ms.p95)}</TableCell><TableCell>{duration(row.ttft_ms.p50)}</TableCell><TableCell>{row.stream_output_tokens_per_second.value === null ? '—' : t('tokensPerSecond', { value: row.stream_output_tokens_per_second.value.toFixed(1) })}</TableCell></TableRow>)}
                    {featureRows.length === 0 && <TableRow><TableCell colSpan={10}>{t('noSamples')}</TableCell></TableRow>}
                </TableBody></Table>
            </>}
        </section>
    );
}
