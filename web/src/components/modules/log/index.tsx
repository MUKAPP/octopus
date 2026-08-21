import { useCallback, useMemo } from 'react';
import { useLogs } from '@/api/endpoints/log';
import { LogCard } from './Item';
import { ActiveRequests } from './ActiveRequests';
import { FileClock, Loader2 } from 'lucide-react';
import { useTranslations } from 'use-intl';
import { VirtualizedGrid } from '@/components/common/VirtualizedGrid';
import { ListRequestError } from '@/components/common/ListRequestError';

/**
 * 日志页面组件
 * - 初始加载 pageSize 条历史日志
 * - SSE 实时推送新日志
 * - 滚动自动加载更多
 */
export function Log() {
    const t = useTranslations('log');
    const { logs, activeRequests, isOverview, hasMore, isLoading, isLoadingMore, isError, listError, isFetching, loadMore, refetch } = useLogs({ pageSize: 10 });
    const commonT = useTranslations('common');
    const hasListError = !isOverview && (isError || Boolean(listError));

    const canLoadMore = hasMore && !isLoading && !isLoadingMore && logs.length > 0;
    const handleReachEnd = useCallback(() => {
        if (!canLoadMore) return;
        void loadMore();
    }, [canLoadMore, loadMore]);

    const footer = useMemo(() => {
        if (hasMore && (isLoading || isLoadingMore)) {
            return (
                <div className="flex justify-center py-4">
                    <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                </div>
            );
        }
        if (!hasMore && logs.length > 0) {
            return (
                <div className="flex justify-center py-4">
                    <span className="text-sm text-muted-foreground">{t('list.noMore')}</span>
                </div>
            );
        }
        return null;
    }, [hasMore, isLoading, isLoadingMore, logs.length, t]);
    return (
        <div className="flex h-full min-h-0 flex-col gap-2">
            <ActiveRequests requests={activeRequests} />
            <div className="relative min-h-0 flex-1">
                <VirtualizedGrid
                    items={logs}
                    isLoading={isLoading && logs.length === 0}
                    layout="list"
                    columns={{ default: 1 }}
                    estimateItemHeight={80}
                    emptyState={
                        hasListError && logs.length === 0 ? (
                            <ListRequestError
                                description={commonT('listRequestError')}
                                retryLabel={commonT('listRetry')}
                                onRetry={refetch}
                                isRetrying={isFetching}
                            />
                        ) : (
                            <div className="flex flex-col items-center gap-3 text-center text-muted-foreground">
                                <FileClock className="size-10 opacity-40" aria-hidden="true" />
                                <p className="text-sm">{t('list.empty')}</p>
                            </div>
                        )
                    }
                    overscan={8}
                    getItemKey={(log) => `log-${log.id}`}
                    renderItem={(log) => <LogCard log={log} />}
                    footer={footer}
                    onReachEnd={handleReachEnd}
                    reachEndEnabled={canLoadMore}
                    reachEndOffset={2}
                />
                {hasListError && logs.length > 0 && (
                    <ListRequestError
                        variant="banner"
                        description={commonT('listRequestError')}
                        retryLabel={commonT('listRetry')}
                        onRetry={refetch}
                        isRetrying={isFetching}
                    />
                )}
            </div>
        </div>
    );
}
