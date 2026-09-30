import { useSyncExternalStore } from 'react';
import type { RelayLog } from '@/api/endpoints/log';

const listeners = new Set<() => void>();
let snapshot = 0;
let timer: number | undefined;

function getSnapshot(): number {
    return snapshot;
}

function getServerSnapshot(): number {
    return 0;
}

function refreshSnapshot() {
    snapshot = performance.now();
    listeners.forEach((listener) => listener());
}

function stopTimer() {
    if (timer === undefined) return;
    window.clearInterval(timer);
    timer = undefined;
}

function syncVisibility() {
    if (document.hidden) {
        stopTimer();
        return;
    }

    refreshSnapshot();
    timer ??= window.setInterval(refreshSnapshot, 1000);
}

function subscribe(listener: () => void) {
    listeners.add(listener);
    if (listeners.size === 1) {
        document.addEventListener('visibilitychange', syncVisibility);
        window.addEventListener('focus', syncVisibility);
        syncVisibility();
    }

    return () => {
        listeners.delete(listener);
        if (listeners.size === 0) {
            stopTimer();
            document.removeEventListener('visibilitychange', syncVisibility);
            window.removeEventListener('focus', syncVisibility);
        }
    };
}

function subscribeInactive() {
    return () => {};
}

/** 可见活动日志共享单调时钟；终态保留服务端耗时。 */
export function useLiveLogDuration(log: RelayLog): number {
    const observedAt = log.duration_observed_at_ms;
    const isActive = log.state === 'running' || log.state === 'committed';
    const shouldSubscribe = isActive && observedAt !== undefined;
    const sharedNow = useSyncExternalStore(
        shouldSubscribe ? subscribe : subscribeInactive,
        shouldSubscribe ? getSnapshot : getServerSnapshot,
        getServerSnapshot,
    );

    if (!isActive || observedAt === undefined) return log.use_time;
    return Math.round(log.use_time + Math.max(0, sharedNow - observedAt));
}
