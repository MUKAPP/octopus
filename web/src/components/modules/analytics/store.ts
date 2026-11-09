import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';
import type { AnalyticsPeriod } from '@/api/endpoints/analytics';

export type AnalyticsStatus = 'all' | 'success' | 'failed';
export type AnalyticsChartMetric = 'cost' | 'count' | 'tokens' | 'input' | 'output' | 'cached' | 'cacheRatio' | 'success' | 'failed' | 'latency';
export type AnalyticsBreakdownTab = 'model' | 'actual-model' | 'api-key' | 'channel';
export type AnalyticsSortMetric = 'cost' | 'count' | 'tokens' | 'latency';

export interface AnalyticsEntityOption {
    id: number;
    name: string;
}

interface AnalyticsViewState {
    period: AnalyticsPeriod;
    /** 持久化值固定 YYYY-MM-DD */
    customStartDate: string | null;
    /** 持久化值固定 YYYY-MM-DD */
    customEndDate: string | null;
    /** null=all，''=unknown */
    model: string | null;
    /** null=all，''=unknown */
    actualModel: string | null;
    apiKey: AnalyticsEntityOption | null;
    channel: AnalyticsEntityOption | null;
    status: AnalyticsStatus;
    stream: 'all' | 'true' | 'false';
    reasoningEffort: string | null;
    taskType: string | null;
    clientProtocol: string | null;
    errorCategory: string | null;
    chartMetric: AnalyticsChartMetric;
    breakdownTab: AnalyticsBreakdownTab;
    sortMetric: AnalyticsSortMetric;
    setPeriod: (value: AnalyticsPeriod) => void;
    setCustomStartDate: (value: string | null) => void;
    setCustomEndDate: (value: string | null) => void;
    setModel: (value: string | null) => void;
    setActualModel: (value: string | null) => void;
    setApiKey: (value: AnalyticsEntityOption | null) => void;
    setChannel: (value: AnalyticsEntityOption | null) => void;
    setStatus: (value: AnalyticsStatus) => void;
    setStream: (value: 'all' | 'true' | 'false') => void;
    setReasoningEffort: (value: string | null) => void;
    setTaskType: (value: string | null) => void;
    setClientProtocol: (value: string | null) => void;
    setErrorCategory: (value: string | null) => void;
    setChartMetric: (value: AnalyticsChartMetric) => void;
    setBreakdownTab: (value: AnalyticsBreakdownTab) => void;
    setSortMetric: (value: AnalyticsSortMetric) => void;
    /** 清除全部筛选条件（保留图表指标、拆分页签与排序） */
    clearFilters: () => void;
}

export const useAnalyticsViewStore = create<AnalyticsViewState>()(
    persist(
        (set) => ({
            period: '30d',
            customStartDate: null,
            customEndDate: null,
            model: null,
            actualModel: null,
            apiKey: null,
            channel: null,
            status: 'all',
            stream: 'all',
            reasoningEffort: null,
            taskType: null,
            clientProtocol: null,
            errorCategory: null,
            chartMetric: 'cost',
            breakdownTab: 'model',
            sortMetric: 'cost',
            setPeriod: (value) => set({ period: value }),
            setCustomStartDate: (value) => set({ customStartDate: value }),
            setCustomEndDate: (value) => set({ customEndDate: value }),
            setModel: (value) => set({ model: value }),
            setActualModel: (value) => set({ actualModel: value }),
            setApiKey: (value) => set({ apiKey: value }),
            setChannel: (value) => set({ channel: value }),
            setStatus: (value) => set({ status: value }),
            setStream: (value) => set({ stream: value }),
            setReasoningEffort: (value) => set({ reasoningEffort: value }),
            setTaskType: (value) => set({ taskType: value }),
            setClientProtocol: (value) => set({ clientProtocol: value }),
            setErrorCategory: (value) => set({ errorCategory: value }),
            setChartMetric: (value) => set({ chartMetric: value }),
            setBreakdownTab: (value) => set({ breakdownTab: value }),
            setSortMetric: (value) => set({ sortMetric: value }),
            clearFilters: () => set({
                period: '30d',
                customStartDate: null,
                customEndDate: null,
                model: null,
                actualModel: null,
                apiKey: null,
                channel: null,
                status: 'all',
            stream: 'all',
            reasoningEffort: null,
            taskType: null,
            clientProtocol: null,
            errorCategory: null,
            }),
        }),
        {
            name: 'analytics-view-options-storage',
            storage: createJSONStorage(() => localStorage),
            partialize: (state) => ({
                period: state.period,
                customStartDate: state.customStartDate,
                customEndDate: state.customEndDate,
                model: state.model,
                actualModel: state.actualModel,
                apiKey: state.apiKey,
                channel: state.channel,
                status: state.status,
                stream: state.stream,
                reasoningEffort: state.reasoningEffort,
                taskType: state.taskType,
                clientProtocol: state.clientProtocol,
                errorCategory: state.errorCategory,
                chartMetric: state.chartMetric,
                breakdownTab: state.breakdownTab,
                sortMetric: state.sortMetric,
            }),
        }
    )
);
