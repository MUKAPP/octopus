import { useMemo } from 'react';
import { PageWrapper } from '@/components/common/PageWrapper';
import { useAnalytics, type AnalyticsFilters } from '@/api/endpoints/analytics';
import { useAnalyticsViewStore } from './store';
import { FilterBar } from './FilterBar';
import { SummaryCards } from './SummaryCards';
import { TrendChart } from './TrendChart';
import { BreakdownTable } from './BreakdownTable';


export function Analytics() {
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

    return (
        <PageWrapper className="h-full min-h-0 overflow-y-auto overscroll-contain space-y-4 pb-24 md:pb-4 rounded-t-3xl">
            <FilterBar query={analyticsQuery} />
            <SummaryCards query={analyticsQuery} />
            <TrendChart query={analyticsQuery} />
            <BreakdownTable query={analyticsQuery} />
        </PageWrapper>
    );
}
