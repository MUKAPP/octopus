import { useEffect } from 'react';
import { useLogOverview } from '@/api/endpoints/log';
import { PageWrapper } from '@/components/common/PageWrapper';
import { Activity } from './activity';
import { HourlyPulseChart } from './HourlyPulseChart';
import { LiveActivityCard } from './LiveActivityCard';
import { OpsInspectorCard } from './OpsInspectorCard';
import { PulseCards } from './PulseCards';
import { useHomeLiveStore } from './store';

export function Home() {
    const { activeLogs, recentLogs, status, refresh } = useLogOverview();
    const setLiveSnapshot = useHomeLiveStore((s) => s.setLiveSnapshot);
    const setRefreshStream = useHomeLiveStore((s) => s.setRefreshStream);
    const resetLiveState = useHomeLiveStore((s) => s.resetLiveState);

    useEffect(() => {
        setLiveSnapshot(status, activeLogs.length);
    }, [status, activeLogs.length, setLiveSnapshot]);

    useEffect(() => {
        setRefreshStream(refresh);
        return () => {
            resetLiveState();
        };
    }, [refresh, setRefreshStream, resetLiveState]);

    return (
        <PageWrapper className="scrollbar h-full min-h-0 overflow-y-auto overscroll-contain space-y-4 md:space-y-5 pb-nav-clearance md:pb-[calc(1rem+var(--safe-area-bottom))] rounded-t-3xl">
            <PulseCards activeCount={activeLogs.length} />
            <HourlyPulseChart />
            <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
                <OpsInspectorCard />
                <LiveActivityCard
                    activeLogs={activeLogs}
                    recentLogs={recentLogs}
                    status={status}
                />
            </div>
            <Activity />
        </PageWrapper>
    );
}
