import { useMemo } from 'react';
import { useTranslations } from 'use-intl';
import { Loader2, Inbox } from 'lucide-react';
import type { UseQueryResult } from '@tanstack/react-query';
import { Tabs, TabsList, TabsTrigger, TabsContents, TabsContent } from '@/components/animate-ui/components/animate/tabs';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import type { AnalyticsMetricsFormatted, AnalyticsResponseFormatted } from '@/api/endpoints/analytics';
import { useAnalyticsViewStore, type AnalyticsBreakdownTab, type AnalyticsSortMetric } from './store';
import { cn } from '@/lib/utils';

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
    AnalyticsMetricsFormatted,
    'total_cost' | 'request_count' | 'total_token' | 'average_wait_time'
>> = {
    cost: 'total_cost',
    count: 'request_count',
    tokens: 'total_token',
    latency: 'average_wait_time',
};

export function BreakdownTable({ query }: BreakdownTableProps) {
    const t = useTranslations('analytics.breakdown');
    const tFeedback = useTranslations('analytics.feedback');
    const data = query.data;

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
                    key: `api-key:${item.id}`,
                    name: item.name,
                    id: item.id,
                    metrics: item.metrics,
                })),
            };
        }
        return {
            kind: 'entity',
            rows: data.by_channel.map((item) => ({
                key: `channel:${item.id}`,
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
        model: t('tab.model'),
        'actual-model': t('tab.actualModel'),
        'api-key': t('tab.apiKey'),
        channel: t('tab.channel'),
    };

    const sortOptions: { value: AnalyticsSortMetric; label: string }[] = [
        { value: 'cost', label: t('totalCost') },
        { value: 'count', label: t('requests') },
        { value: 'tokens', label: t('totalTokens') },
        { value: 'latency', label: t('avgWaitTime') },
    ];

    const sortableHead = (column: AnalyticsSortMetric, label: string) => (
        <TableHead aria-sort={sortMetric === column ? 'descending' : 'none'}>
            <button
                type="button"
                onClick={() => setSortMetric(column)}
                className={cn(
                    'text-left font-medium transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
                    sortMetric === column ? 'text-foreground' : 'text-muted-foreground'
                )}
            >
                {label}
            </button>
        </TableHead>
    );

    const renderMetric = (metric: FormattedMetric) => (
        <span className="tabular-nums">
            {metric.formatted.value}
            <span className="ml-0.5 text-xs text-muted-foreground">{metric.formatted.unit}</span>
        </span>
    );

    const renderList = () => {
        if (query.isLoading && !data) {
            return (
                <div className="flex items-center justify-center py-8">
                    <Loader2 className="size-8 animate-spin text-muted-foreground" role="status" aria-label={tFeedback('loading')} />
                </div>
            );
        }
        if (query.isError && !data) {
            return (
                <div role="alert" className="flex flex-col items-center justify-center gap-3 py-8 text-sm text-muted-foreground">
                    <p>{tFeedback('loadFailed')}</p>
                    <button type="button" onClick={() => void query.refetch()} className="rounded-xl border border-border px-3 py-1.5 font-medium text-foreground transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring">
                        {tFeedback('retry')}
                    </button>
                </div>
            );
        }
        if (sortedRows.length === 0) {
            return (
                <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
                    <Inbox className="w-12 h-12 mb-3 opacity-30" />
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
                            {sortedRows.map((row) => (
                                <TableRow key={row.key}>
                                    <TableCell className="font-medium">{displayName(row)}</TableCell>
                                    <TableCell>{renderMetric(row.metrics.request_count)}</TableCell>
                                    <TableCell className="text-right tabular-nums">{row.metrics.success_rate.formatted}</TableCell>
                                    <TableCell>{renderMetric(row.metrics.total_token)}</TableCell>
                                    <TableCell className="text-right">{renderMetric(row.metrics.cached_token)}</TableCell>
                                    <TableCell>{renderMetric(row.metrics.total_cost)}</TableCell>
                                    <TableCell>{renderMetric(row.metrics.average_wait_time)}</TableCell>
                                </TableRow>
                            ))}
                        </TableBody>
                    </Table>
                </div>
                {/* 小屏堆叠行 */}
                <div className="space-y-3 md:hidden">
                    {sortedRows.map((row) => (
                        <div key={row.key} className="flex items-center gap-3 p-3 rounded-2xl hover:bg-accent/5 transition-colors">
                            <div className="flex-1 min-w-0">
                                <p className="font-medium text-sm truncate">{displayName(row)}</p>
                                <div className="flex items-center gap-1 text-xs text-muted-foreground mt-1">
                                    <span>{t('successRate')}:</span>
                                    <span className="tabular-nums">{row.metrics.success_rate.formatted}</span>
                                </div>
                            </div>
                            <div className="flex items-center gap-1 text-right shrink-0">
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
                    ))}
                </div>
            </>
        );
    };

    return (
        <div className="rounded-3xl bg-card text-card-foreground border-card-border border p-4">
            <Tabs value={breakdownTab} onValueChange={(value) => setBreakdownTab(value as AnalyticsBreakdownTab)}>
                <div className="flex flex-wrap items-center justify-between gap-3">
                    <TabsList>
                        {(Object.keys(tabLabels) as AnalyticsBreakdownTab[]).map((tab) => (
                            <TabsTrigger key={tab} value={tab}>{tabLabels[tab]}</TabsTrigger>
                        ))}
                    </TabsList>
                    <div className="ml-auto flex items-center gap-2">
                        <Select value={sortMetric} onValueChange={(value) => setSortMetric(value as AnalyticsSortMetric)}>
                            <SelectTrigger aria-label={sortOptions.find((option) => option.value === sortMetric)?.label} size="sm" className="min-w-36 rounded-xl">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent className="rounded-xl">
                                {sortOptions.map((option) => (
                                    <SelectItem className="rounded-xl" key={option.value} value={option.value}>{option.label}</SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                    </div>
                </div>
                {query.isError && data && (
                    <div role="alert" className="mt-3 flex items-center justify-between gap-3 rounded-2xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-muted-foreground">
                        <span>{tFeedback('loadFailed')}</span>
                        <button type="button" onClick={() => void query.refetch()} className="shrink-0 rounded-xl border border-border px-3 py-1.5 font-medium text-foreground transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring">
                            {tFeedback('retry')}
                        </button>
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
