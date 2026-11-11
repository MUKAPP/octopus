import { useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslations } from 'use-intl';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/animate-ui/components/animate/tooltip';
import { cn } from '@/lib/utils';
import { useHomeLiveStore } from './store';

const HOME_QUERY_KEYS = [
    ['stats', 'today'],
    ['stats', 'total'],
    ['stats', 'hourly'],
    ['stats', 'daily'],
    ['channels', 'list'],
    ['apikeys', 'list'],
    ['stats', 'apikey'],
    ['models', 'list'],
] as const;

export function HomeHeaderActions() {
    const t = useTranslations('toolbar.home');
    const queryClient = useQueryClient();
    const status = useHomeLiveStore((s) => s.status);
    const activeCount = useHomeLiveStore((s) => s.activeCount);
    const refreshStream = useHomeLiveStore((s) => s.refreshStream);
    const [isRefreshing, setIsRefreshing] = useState(false);

    const statusConfig = {
        connected: {
            label: t('live'),
            tooltip: t('liveTooltip'),
            dotClass: activeCount > 0 ? 'bg-emerald-500 animate-pulse motion-reduce:animate-none' : 'bg-emerald-500',
            textClass: 'text-foreground',
        },
        connecting: {
            label: t('connecting'),
            tooltip: t('connectingTooltip'),
            dotClass: 'bg-amber-500 animate-pulse motion-reduce:animate-none',
            textClass: 'text-muted-foreground',
        },
        reconnecting: {
            label: t('reconnecting'),
            tooltip: t('reconnectingTooltip'),
            dotClass: 'bg-amber-500 animate-pulse motion-reduce:animate-none',
            textClass: 'text-amber-600 dark:text-amber-400',
        },
        fallback: {
            label: t('fallback'),
            tooltip: t('fallbackTooltip'),
            dotClass: 'bg-muted-foreground/70',
            textClass: 'text-muted-foreground',
        },
    }[status];

    const handleRefresh = async () => {
        if (isRefreshing) return;
        setIsRefreshing(true);
        try {
            await Promise.allSettled([
                ...HOME_QUERY_KEYS.map((queryKey) => queryClient.refetchQueries({ queryKey: [...queryKey] })),
                refreshStream ? refreshStream() : Promise.resolve(),
            ]);
        } finally {
            setIsRefreshing(false);
        }
    };

    return (
        <TooltipProvider>
            <div className="flex items-center gap-2">
                <Tooltip>
                    <TooltipTrigger asChild>
                        <div
                            role="status"
                            aria-live="polite"
                            aria-label={`${statusConfig.label}: ${statusConfig.tooltip}`}
                            className="inline-flex h-8 sm:h-9 items-center gap-2 rounded-full border border-border bg-card/85 px-3 text-xs font-medium shadow-2xs select-none"
                        >
                            <span className={cn('size-2 rounded-full shrink-0', statusConfig.dotClass)} aria-hidden="true" />
                            <span className={cn('whitespace-nowrap', statusConfig.textClass)}>{statusConfig.label}</span>
                        </div>
                    </TooltipTrigger>
                    <TooltipContent>{statusConfig.tooltip}</TooltipContent>
                </Tooltip>

                <Tooltip>
                    <TooltipTrigger asChild>
                        <button
                            type="button"
                            onClick={() => void handleRefresh()}
                            disabled={isRefreshing}
                            aria-label={t('refresh')}
                            className="inline-flex size-8 sm:size-9 items-center justify-center rounded-full border border-border bg-card/85 text-muted-foreground shadow-2xs transition-colors motion-reduce:transition-none hover:bg-muted/60 hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:opacity-60"
                        >
                            <RefreshCw className={cn('size-4', isRefreshing && 'animate-spin motion-reduce:animate-none')} aria-hidden="true" />
                        </button>
                    </TooltipTrigger>
                    <TooltipContent>{t('refresh')}</TooltipContent>
                </Tooltip>
            </div>
        </TooltipProvider>
    );
}
