import { useCallback } from 'react';
import { CONTENT_MAP } from './config';

export function usePreload() {
    const preload = useCallback((routeId: string) => {
        const component = CONTENT_MAP[routeId];
        if (component?.preload) {
            void component.preload().catch(() => undefined);
        }
    }, []);

    return { preload };
}
