import { useSyncExternalStore } from 'react';

export type SafeAreaInsets = {
    top: number;
    right: number;
    bottom: number;
    left: number;
};

const serverInsets: SafeAreaInsets = { top: 0, right: 0, bottom: 0, left: 0 };
const listeners = new Set<() => void>();
let snapshot: SafeAreaInsets | undefined;
let removeViewportListeners: (() => void) | undefined;

export function getSafeAreaInsets(): SafeAreaInsets {
    const style = getComputedStyle(document.documentElement);
    return {
        top: parseFloat(style.getPropertyValue('--safe-area-top')),
        right: parseFloat(style.getPropertyValue('--safe-area-right')),
        bottom: parseFloat(style.getPropertyValue('--safe-area-bottom')),
        left: parseFloat(style.getPropertyValue('--safe-area-left')),
    };
}

function getSnapshot(): SafeAreaInsets {
    snapshot ??= getSafeAreaInsets();
    return snapshot;
}

function getServerSnapshot(): SafeAreaInsets {
    return serverInsets;
}

function refreshSnapshot() {
    const next = getSafeAreaInsets();
    if (snapshot &&
        snapshot.top === next.top &&
        snapshot.right === next.right &&
        snapshot.bottom === next.bottom &&
        snapshot.left === next.left) return;

    snapshot = next;
    listeners.forEach((listener) => listener());
}

function subscribe(listener: () => void) {
    listeners.add(listener);
    if (listeners.size === 1) {
        const viewport = window.visualViewport;
        window.addEventListener('resize', refreshSnapshot);
        viewport?.addEventListener('resize', refreshSnapshot);
        removeViewportListeners = () => {
            window.removeEventListener('resize', refreshSnapshot);
            viewport?.removeEventListener('resize', refreshSnapshot);
        };
        refreshSnapshot();
    }

    return () => {
        listeners.delete(listener);
        if (listeners.size === 0) {
            removeViewportListeners?.();
            removeViewportListeners = undefined;
            snapshot = undefined;
        }
    };
}

export function useSafeAreaInsets(): SafeAreaInsets {
    return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}

export function safeAreaCollisionPadding(
    insets: SafeAreaInsets,
    padding: number | Partial<SafeAreaInsets> = 0,
): SafeAreaInsets {
    return {
        top: insets.top + (typeof padding === 'number' ? padding : padding.top ?? 0),
        right: insets.right + (typeof padding === 'number' ? padding : padding.right ?? 0),
        bottom: insets.bottom + (typeof padding === 'number' ? padding : padding.bottom ?? 0),
        left: insets.left + (typeof padding === 'number' ? padding : padding.left ?? 0),
    };
}
