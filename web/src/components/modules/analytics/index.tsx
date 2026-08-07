import { useMemo } from 'react';
import { useTranslations } from 'use-intl';
import { PageWrapper } from '@/components/common/PageWrapper';
import { useAnalytics, type AnalyticsFilters } from '@/api/endpoints/analytics';
import { useAnalyticsViewStore } from './store';
import { FilterBar } from './FilterBar';
import { SummaryCards } from './SummaryCards';
import { TrendChart } from './TrendChart';
import { BreakdownTable } from './BreakdownTable';

/**
 * 将后端 YYYYMMDD 转为展示用 YYYY-MM-DD
 */
function formatDisplayDate(value: string): string {
    if (value.length !== 8) return value;
    return `${value.slice(0, 4)}-${value.slice(4, 6)}-${value.slice(6, 8)}`;
}

export function Analytics() {
    const t = useTranslations('analytics');

    const period = useAnalyticsViewStore((state) => state.period);
    const customStartDate = useAnalyticsViewStore((state) => state.customStartDate);
    const customEndDate = useAnalyticsViewStore((state) => state.customEndDate);
    const model = useAnalyticsViewStore((state) => state.model);
    const actualModel = useAnalyticsViewStore((state) => state.actualModel);
    const apiKey = useAnalyticsViewStore((state) => state.apiKey);
    const channel = useAnalyticsViewStore((state) => state.channel);
    const status = useAnalyticsViewStore((state) => state.status);

    const filters = useMemo<AnalyticsFilters>(() => ({
        period,
        customStartDate,
        customEndDate,
        model,
        actualModel,
        apiKey,
        channel,
        status,
    }), [period, customStartDate, customEndDate, model, actualModel, apiKey, channel, status]);

    const analyticsQuery = useAnalytics(filters);
    const data = analyticsQuery.data;

    const showResolvedRange = Boolean(data?.resolved_start_date && data.resolved_end_date);

    return (
        <PageWrapper className="h-full min-h-0 overflow-y-auto overscroll-contain space-y-6 pb-24 md:pb-4 rounded-t-3xl">
            <FilterBar query={analyticsQuery} />
            {(data?.available_from || showResolvedRange) && (
                <div className="px-1 text-xs text-muted-foreground flex flex-col gap-0.5">
                    {data?.available_from && (
                        <p>{t('availableFrom', { date: formatDisplayDate(data.available_from) })}</p>
                    )}
                    {showResolvedRange && data && (
                        <p>
                            {t('resolvedRange', {
                                start: formatDisplayDate(data.resolved_start_date),
                                end: formatDisplayDate(data.resolved_end_date),
                            })}
                        </p>
                    )}
                </div>
            )}
            <SummaryCards query={analyticsQuery} />
            <TrendChart query={analyticsQuery} />
            <BreakdownTable query={analyticsQuery} />
        </PageWrapper>
    );
}
