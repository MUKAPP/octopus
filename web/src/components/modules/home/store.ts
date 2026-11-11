import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';
import type { LogStreamStatus } from '@/api/endpoints/log';

export type ChartMetricType = 'cost' | 'count' | 'tokens';

interface HomeViewState {
    chartMetricType: ChartMetricType;
    setChartMetricType: (value: ChartMetricType) => void;
}

export const useHomeViewStore = create<HomeViewState>()(
    persist(
        (set) => ({
            chartMetricType: 'cost',
            setChartMetricType: (value) => set({ chartMetricType: value }),
        }),
        {
            name: 'home-view-options-storage',
            storage: createJSONStorage(() => localStorage),
            partialize: (state) => ({
                chartMetricType: state.chartMetricType,
            }),
        }
    )
);

interface HomeLiveState {
    status: LogStreamStatus;
    activeCount: number;
    refreshStream: (() => Promise<void>) | null;
    setLiveSnapshot: (status: LogStreamStatus, activeCount: number) => void;
    setRefreshStream: (fn: (() => Promise<void>) | null) => void;
    resetLiveState: () => void;
}

export const useHomeLiveStore = create<HomeLiveState>()((set) => ({
    status: 'connecting',
    activeCount: 0,
    refreshStream: null,
    setLiveSnapshot: (status, activeCount) => set({ status, activeCount }),
    setRefreshStream: (refreshStream) => set({ refreshStream }),
    resetLiveState: () => set({ status: 'connecting', activeCount: 0, refreshStream: null }),
}));
