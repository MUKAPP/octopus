import { Suspense, useMemo, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { useTranslations } from 'use-intl';
import { CONTENT_MAP } from './config';
import { RouteErrorBoundary } from './route-error-boundary';

type RetryState = {
    routeId: string;
    attempt: number;
};

export function ContentLoader({ activeRoute }: { activeRoute: string }) {
    const t = useTranslations('route');
    const [retryState, setRetryState] = useState<RetryState>({
        routeId: activeRoute,
        attempt: 0,
    });
    const routeComponent = CONTENT_MAP[activeRoute];
    const retryAttempt = retryState.routeId === activeRoute ? retryState.attempt : 0;
    const Component = useMemo(
        () => (retryAttempt === 0 ? routeComponent : routeComponent?.retry()),
        [retryAttempt, routeComponent],
    );

    const retry = () => {
        setRetryState((current) => ({
            routeId: activeRoute,
            attempt: current.routeId === activeRoute ? current.attempt + 1 : 1,
        }));
    };

    if (!Component) {
        return (
            <div className="flex h-full min-h-64 items-center justify-center px-4">
                <p className="text-muted-foreground">{t('notFound')}</p>
            </div>
        );
    }

    return (
        <RouteErrorBoundary
            key={`${activeRoute}-${retryAttempt}`}
            fallback={
                <div className="flex h-full min-h-64 flex-col items-center justify-center gap-3 px-4" role="alert">
                    <p className="text-sm text-destructive">{t('loadFailed')}</p>
                    <button
                        type="button"
                        className="rounded-lg bg-primary px-3 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        onClick={retry}
                    >
                        {t('retry')}
                    </button>
                </div>
            }
        >
            <Suspense
                fallback={
                    <div className="flex h-full min-h-64 items-center justify-center gap-2 px-4" role="status">
                        <Loader2 className="size-5 animate-spin text-muted-foreground" aria-hidden="true" />
                        <span className="text-sm text-muted-foreground">{t('loading')}</span>
                    </div>
                }
            >
                <Component />
            </Suspense>
        </RouteErrorBoundary>
    );
}
