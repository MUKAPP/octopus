import { useSyncExternalStore } from 'react';

const query = '(prefers-reduced-motion: reduce)';

function subscribe(listener: () => void) {
    const mediaQuery = window.matchMedia(query);
    mediaQuery.addEventListener('change', listener);
    return () => mediaQuery.removeEventListener('change', listener);
}

function getSnapshot() {
    return window.matchMedia(query).matches;
}

// 服务端尚未取得用户偏好时先保持静态。
function getServerSnapshot() {
    return true;
}

/** 同时响应初始偏好和页面打开后的系统设置变更。 */
export function usePrefersReducedMotion() {
    return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}
