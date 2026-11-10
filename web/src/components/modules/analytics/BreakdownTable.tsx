import { Fragment, useMemo, useState } from 'react';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import { useTranslations } from 'use-intl';
import { ArrowDown, Loader2, Inbox, ChevronRight } from 'lucide-react';
import type { UseQueryResult } from '@tanstack/react-query';
import { Tabs, TabsList, TabsTrigger, TabsContents, TabsContent } from '@/components/animate-ui/components/animate/tabs';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { analyticsEntityKey, type AnalyticsMetricsFormatted, type AnalyticsResponseFormatted } from '@/api/endpoints/analytics';
import { useAnalyticsViewStore, type AnalyticsBreakdownTab, type AnalyticsSortMetric } from './store';
import { cn } from '@/lib/utils';
import { InlineRetry } from '@/components/common/InlineRetry';
import { EASING, REDUCED_MOTION_TRANSITION } from '@/lib/animations/fluid-transitions';

interface BreakdownTableProps {
    query: UseQueryResult<AnalyticsResponseFormatted>;
}

interface BreakdownRow {
    key: string;
    name: string;
    id?: number;
    metrics: AnalyticsMetricsFormatted;
}

type BreakdownKind = 'model' | 'entity';

type FormattedMetric = AnalyticsMetricsFormatted['total_cost'];

const SORT_METRIC_KEY: Record<AnalyticsSortMetric, keyof Pick<
    AnalyticsMetricsFormatted, 'total_cost' | 'request_count' | 'total_token' | 'average_wait_time'
>> = {
    cost: 'total_cost',
    count: 'request_count',
    tokens: 'total_token',
    latency: 'average_wait_time',
};

export function BreakdownTable({ query }: BreakdownTableProps) {
    const t = useTranslations('analytics.breakdown');
    const tFeedback = useTranslations('analytics.feedback');
    const tDetail = useTranslations('analytics.detail');
    const data = query.data;
    const shouldReduceMotion = useReducedMotion() ?? false;
    const [expandedKeys, setExpandedKeys] = useState<Record<string, boolean>>({});

    const toggleRow = (key: string) => {
        setExpandedKeys((prev) => ({ ...prev, [key]: !prev[key] }));
    };

    const breakdownTab = useAnalyticsViewStore((state) => state.breakdownTab);
    const setBreakdownTab = useAnalyticsViewStore((state) => state.setBreakdownTab);
    const sortMetric = useAnalyticsViewStore((state) => state.sortMetric);
    const setSortMetric = useAnalyticsViewStore((state) => state.setSortMetric);

    const { rows, kind } = useMemo<{ rows: BreakdownRow[]; kind: BreakdownKind }>(() => {
        if (!data) return { rows: [], kind: 'model' };
        if (breakdownTab === 'model') {
            return {
                kind: 'model',
                rows: data.by_model.map((item) => ({
                    key: `model:${item.name}`,
                    name: item.name,
                    metrics: item.metrics,
                })),
            };
        }
        if (breakdownTab === 'actual-model') {
            return {
                kind: 'model',
                rows: data.by_actual_model.map((item) => ({
                    key: `actual-model:${item.name}`,
                    name: item.name,
                    metrics: item.metrics,
                })),
            };
        }
        if (breakdownTab === 'api-key') {
            return {
                kind: 'entity',
                rows: data.by_api_key.map((item) => ({
                    key: `api-key:${analyticsEntityKey(item)}`,
                    name: item.name,
                    id: item.id,
                    metrics: item.metrics,
                })),
            };
        }
        return {
            kind: 'entity',
            rows: data.by_channel.map((item) => ({
                key: `channel:${analyticsEntityKey(item)}`,
                name: item.name,
                id: item.id,
                metrics: item.metrics,
            })),
        };
    }, [data, breakdownTab]);

    const sortedRows = useMemo(() => {
        const sortKey = SORT_METRIC_KEY[sortMetric];
        return [...rows].sort((a, b) => {
            const metricDiff = b.metrics[sortKey].raw - a.metrics[sortKey].raw;
            if (metricDiff !== 0) return metricDiff;
            const nameDiff = a.name.localeCompare(b.name);
            if (nameDiff !== 0) return nameDiff;
            return (a.id ?? 0) - (b.id ?? 0);
        });
    }, [rows, sortMetric]);

    const displayName = (row: BreakdownRow): string => {
        if (kind === 'entity') {
            return row.name || t('unassigned');
        }
        return row.name || t('unknown');
    };

    const tabLabels: Record<AnalyticsBreakdownTab, string> = {
        model: t('tab.model'), 'actual-model': t('tab.actualModel'), 'api-key': t('tab.apiKey'),
        channel: t('tab.channel'),
    };

    const sortOptions: { value: AnalyticsSortMetric; label: string }[] = [
        { value: 'cost', label: t('totalCost') },
        { value: 'count', label: t('requests') },
        { value: 'tokens', label: t('totalTokens') },
        { value: 'latency', label: t('avgWaitTime') },
    ];

    const sortableHead = (column: AnalyticsSortMetric, label: string) => {
        const isActive = sortMetric === column;
        return (
            <TableHead aria-sort={isActive ? 'descending' : 'none'} className="text-right">
                <button
                    type="button"
                    onClick={() => setSortMetric(column)}
                    className={cn(
                        'inline-flex items-center justify-end gap-1 font-medium transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
                        isActive ? 'text-foreground' : 'text-muted-foreground',
                    )}
                >
                    <span>{label}</span>
                    {isActive && <ArrowDown className="size-3.5 text-primary" aria-hidden="true" />}
                </button>
            </TableHead>
        );
    };

    const renderMetric = (metric: FormattedMetric) => (
        <span className="tabular-nums">
            {metric.formatted.value}
            <span className="ml-0.5 text-xs text-muted-foreground">{metric.formatted.unit}</span>
        </span>
    );

    const getDetailMetrics = (row: BreakdownRow) => {
        const totalCost = data?.summary.total_cost.raw ?? 0;
        return [
            { label: tDetail('inputTokens'), value: renderMetric(row.metrics.input_token) },
            { label: tDetail('outputTokens'), value: renderMetric(row.metrics.output_token) },
            { label: tDetail('cacheRead'), value: row.metrics.observed_cached_token === null ? '—' : renderMetric(row.metrics.observed_cached_token) },
            { label: tDetail('cacheRatio'), value: row.metrics.cache_read_ratio.formatted },
            { label: tDetail('inputCost'), value: renderMetric(row.metrics.input_cost) },
            { label: tDetail('outputCost'), value: renderMetric(row.metrics.output_cost) },
            { label: tDetail('averageCost'), value: row.metrics.request_count.raw > 0 ? renderMetric(row.metrics.average_cost) : '—' },
            { label: tDetail('costShare'), value: totalCost > 0 ? (row.metrics.total_cost.raw / totalCost * 100).toFixed(1) + '%' : '—' },
        ];
    };

    const renderList = () => {
        if (query.isLoading && !data) {
            return (
                <div className="flex items-center justify-center py-8">
                    <Loader2 className="size-8 animate-spin motion-reduce:animate-none text-muted-foreground" role="status" aria-label={tFeedback('loading')} />
                </div>
            );
        }
        if (query.isError && !data) {
            return (
                <div role="alert" className="flex flex-col items-center justify-center gap-3 py-8 text-sm text-muted-foreground">
                    <p>{tFeedback('loadFailed')}</p>
                    <InlineRetry onRetry={() => void query.refetch()} label={tFeedback('retry')} />
                </div>
            );
        }
        if (sortedRows.length === 0) {
            return (
                <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
                    <Inbox className="size-10 opacity-40" aria-hidden="true" />
                    <p className="text-sm">{t('empty')}</p>
                </div>
            );
        }
        return (
            <>
                {/* 桌面表格 */}
                <div className="hidden md:block">
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead>{t('name')}</TableHead>
                                {sortableHead('count', t('requests'))}
                                <TableHead className="text-right">{t('successRate')}</TableHead>
                                {sortableHead('tokens', t('totalTokens'))}
                                <TableHead className="text-right">{t('cachedTokens')}</TableHead>
                                {sortableHead('cost', t('totalCost'))}
                                {sortableHead('latency', t('avgWaitTime'))}
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {sortedRows.map((row) => {
                                const isExpanded = Boolean(expandedKeys[row.key]);
                                const detailMetrics = isExpanded ? getDetailMetrics(row) : [];
                                return (
                                    <Fragment key={row.key}>
                                        <TableRow
                                            className={cn(
                                                isExpanded && 'border-b-0 bg-muted/25 hover:bg-muted/35',
                                            )}
                                        >
                                            <TableCell className="py-3 font-medium">
                                                <div className="flex flex-col items-start gap-1">
                                                    <span className="break-all">{displayName(row)}</span>
                                                    <button
                                                        type="button"
                                                        aria-expanded={isExpanded}
                                                        onClick={() => toggleRow(row.key)}
                                                        className={cn(
                                                            '-ml-1.5 inline-flex items-center gap-1 rounded-lg px-1.5 py-0.5 text-xs font-normal transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
                                                            isExpanded
                                                                ? 'bg-background/80 text-foreground'
                                                                : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground',
                                                        )}
                                                    >
                                                        <ChevronRight
                                                            className={cn(
                                                                'size-3.5 transition-transform duration-200 motion-reduce:transition-none',
                                                                isExpanded && 'rotate-90',
                                                            )}
                                                            aria-hidden="true"
                                                        />
                                                        <span>{t('usageDetails')}</span>
                                                    </button>
                                                </div>
                                            </TableCell>
                                            <TableCell className="text-right">{renderMetric(row.metrics.request_count)}</TableCell>
                                            <TableCell className="text-right">
                                                <span className={cn(
                                                    'tabular-nums',
                                                    row.metrics.success_rate.raw < 80
                                                        ? 'text-destructive'
                                                        : row.metrics.success_rate.raw < 95
                                                            ? 'text-amber-600 dark:text-amber-400'
                                                            : '',
                                                )}>
                                                    {row.metrics.success_rate.formatted}
                                                </span>
                                            </TableCell>
                                            <TableCell className="text-right">{renderMetric(row.metrics.total_token)}</TableCell>
                                            <TableCell className="text-right">{row.metrics.observed_cached_token === null ? <span className="text-muted-foreground">—</span> : renderMetric(row.metrics.observed_cached_token)}</TableCell>
                                            <TableCell className="text-right">{renderMetric(row.metrics.total_cost)}</TableCell>
                                            <TableCell className="text-right">{renderMetric(row.metrics.average_wait_time)}</TableCell>
                                        </TableRow>
                                        <AnimatePresence initial={false}>
                                            {isExpanded && (
                                                <TableRow
                                                    key={`${row.key}-details`}
                                                    className="bg-muted/20 hover:bg-muted/20"
                                                >
                                                    <TableCell colSpan={7} className="p-0">
                                                        <motion.div
                                                            initial={shouldReduceMotion ? { opacity: 0 } : { height: 0, opacity: 0, filter: 'blur(4px)' }}
                                                            animate={shouldReduceMotion ? { opacity: 1 } : { height: 'auto', opacity: 1, filter: 'blur(0px)' }}
                                                            exit={shouldReduceMotion ? { opacity: 0 } : { height: 0, opacity: 0, filter: 'blur(4px)' }}
                                                            transition={shouldReduceMotion ? REDUCED_MOTION_TRANSITION : { duration: 0.22, ease: EASING.easeOutCubic }}
                                                            className="overflow-hidden"
                                                        >
                                                            <dl className="grid grid-cols-2 gap-2.5 p-3 lg:grid-cols-4">
                                                                {detailMetrics.map((item) => (
                                                                    <div
                                                                        key={item.label}
                                                                        className="min-w-0 rounded-2xl border border-border/70 bg-background/80 px-3.5 py-2.5"
                                                                    >
                                                                        <dt className="truncate text-xs text-muted-foreground">{item.label}</dt>
                                                                        <dd className="mt-1 text-sm font-semibold tabular-nums">{item.value}</dd>
                                                                    </div>
                                                                ))}
                                                            </dl>
                                                        </motion.div>
                                                    </TableCell>
                                                </TableRow>
                                            )}
                                        </AnimatePresence>
                                    </Fragment>
                                );
                            })}
                        </TableBody>
                    </Table>
                </div>
                {/* 小屏堆叠行 */}
                <div className="space-y-2.5 md:hidden">
                    {sortedRows.map((row) => {
                        const isExpanded = Boolean(expandedKeys[row.key]);
                        const detailMetrics = isExpanded ? getDetailMetrics(row) : [];
                        return (
                            <div key={row.key} className="rounded-2xl border border-border/70 bg-background/80 p-3.5">
                                <div className="flex items-start justify-between gap-3">
                                    <div className="min-w-0 flex-1">
                                        <p className="truncate text-sm font-medium">{displayName(row)}</p>
                                        <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
                                            <span>
                                                {t('successRate')}:{' '}
                                                <span className={cn(
                                                    'tabular-nums font-medium',
                                                    row.metrics.success_rate.raw < 80
                                                        ? 'text-destructive'
                                                        : row.metrics.success_rate.raw < 95
                                                            ? 'text-amber-600 dark:text-amber-400'
                                                            : 'text-foreground',
                                                )}>{row.metrics.success_rate.formatted}</span>
                                            </span>
                                            <span>·</span>
                                            <button
                                                type="button"
                                                aria-expanded={isExpanded}
                                                onClick={() => toggleRow(row.key)}
                                                className="inline-flex items-center gap-0.5 rounded-md text-xs text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                                            >
                                                <ChevronRight
                                                    className={cn(
                                                        'size-3.5 transition-transform duration-200 motion-reduce:transition-none',
                                                        isExpanded && 'rotate-90',
                                                    )}
                                                    aria-hidden="true"
                                                />
                                                <span>{t('usageDetails')}</span>
                                            </button>
                                        </div>
                                    </div>
                                    <div className="shrink-0 text-right font-semibold">
                                        {sortMetric === 'count' ? (
                                            renderMetric(row.metrics.request_count)
                                        ) : sortMetric === 'tokens' ? (
                                            renderMetric(row.metrics.total_token)
                                        ) : sortMetric === 'latency' ? (
                                            renderMetric(row.metrics.average_wait_time)
                                        ) : (
                                            renderMetric(row.metrics.total_cost)
                                        )}
                                    </div>
                                </div>
                                <AnimatePresence initial={false}>
                                    {isExpanded && (
                                        <motion.div
                                            initial={shouldReduceMotion ? { opacity: 0 } : { height: 0, opacity: 0, filter: 'blur(4px)' }}
                                            animate={shouldReduceMotion ? { opacity: 1 } : { height: 'auto', opacity: 1, filter: 'blur(0px)' }}
                                            exit={shouldReduceMotion ? { opacity: 0 } : { height: 0, opacity: 0, filter: 'blur(4px)' }}
                                            transition={shouldReduceMotion ? REDUCED_MOTION_TRANSITION : { duration: 0.22, ease: EASING.easeOutCubic }}
                                            className="overflow-hidden"
                                        >
                                            <div className="pt-3">
                                                <dl className="grid grid-cols-2 gap-2 border-t border-border/50 pt-3">
                                                    {detailMetrics.map((item) => (
                                                        <div key={item.label} className="min-w-0 rounded-xl border border-border/50 bg-muted/25 px-2.5 py-2">
                                                            <dt className="truncate text-[11px] text-muted-foreground">{item.label}</dt>
                                                            <dd className="mt-0.5 text-xs font-semibold tabular-nums">{item.value}</dd>
                                                        </div>
                                                    ))}
                                                </dl>
                                            </div>
                                        </motion.div>
                                    )}
                                </AnimatePresence>
                            </div>
                        );
                    })}
                </div>
            </>
        );
    };

    return (
        <div className="rounded-3xl bg-card text-card-foreground border-border border p-4">
            <Tabs value={breakdownTab} onValueChange={(value) => setBreakdownTab(value as AnalyticsBreakdownTab)}>
                <div className="flex flex-wrap items-center justify-between gap-3">
                    <TabsList>
                        {(Object.keys(tabLabels) as AnalyticsBreakdownTab[]).map((tab) => (
                            <TabsTrigger key={tab} value={tab}>{tabLabels[tab]}</TabsTrigger>
                        ))}
                    </TabsList>
                    <div className="ml-auto flex items-center gap-2">
                        <Select value={sortMetric} onValueChange={(value) => setSortMetric(value as AnalyticsSortMetric)}>
                            <SelectTrigger aria-label={sortOptions.find((option) => option.value === sortMetric)?.label} size="sm" className="min-w-36">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                {sortOptions.map((option) => (
                                    <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                    </div>
                </div>
                {query.isError && data && (
                    <div role="alert" className="mt-3 flex items-center justify-between gap-3 rounded-2xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-muted-foreground">
                        <span>{tFeedback('loadFailed')}</span>
                        <InlineRetry onRetry={() => void query.refetch()} label={tFeedback('retry')} className="shrink-0" />
                    </div>
                )}
                <TabsContents>
                    <TabsContent value="model">
                        {renderList()}
                    </TabsContent>
                    <TabsContent value="actual-model">
                        {renderList()}
                    </TabsContent>
                    <TabsContent value="api-key">
                        {renderList()}
                    </TabsContent>
                    <TabsContent value="channel">
                        {renderList()}
                    </TabsContent>
                </TabsContents>
            </Tabs>
        </div>
    );
}
