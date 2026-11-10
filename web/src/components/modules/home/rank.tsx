import { useChannelList } from '@/api/channel';
import { useMemo } from 'react';
import { useTranslations } from 'use-intl';
import { Loader2, Medal, TrendingUp } from 'lucide-react';
import { Tabs, TabsList, TabsTrigger, TabsContents, TabsContent } from '@/components/animate-ui/components/animate/tabs';
import { useHomeViewStore, type RankSortMode } from '@/components/modules/home/store';
import { InlineRetry } from '@/components/common/InlineRetry';

type ChannelData = NonNullable<ReturnType<typeof useChannelList>['data']>[number];

export function Rank() {
    const { data: channelData, isLoading, isError, refetch } = useChannelList();
    const t = useTranslations('home.rank');
    const tHome = useTranslations('home');
    const rankSortMode = useHomeViewStore((state) => state.rankSortMode);
    const setRankSortMode = useHomeViewStore((state) => state.setRankSortMode);

    const rankedByCost = useMemo<ChannelData[]>(() => {
        if (!channelData) return [];
        return [...channelData].sort((a, b) => b.formatted.total_cost.raw - a.formatted.total_cost.raw);
    }, [channelData]);

    const rankedByCount = useMemo<ChannelData[]>(() => {
        if (!channelData) return [];
        return [...channelData].sort((a, b) => b.formatted.request_count.raw - a.formatted.request_count.raw);
    }, [channelData]);

    const rankedByTokens = useMemo<ChannelData[]>(() => {
        if (!channelData) return [];
        return [...channelData].sort((a, b) => b.formatted.total_token.raw - a.formatted.total_token.raw);
    }, [channelData]);

    const getRankBadge = (rank: number) => {
        if (rank === 1) {
            return (
                <div className="flex size-8 items-center justify-center rounded-xl bg-amber-500/15 text-amber-600 dark:text-amber-400 shrink-0" aria-label="1">
                    <Medal className="size-4" aria-hidden="true" />
                </div>
            );
        }
        if (rank === 2) {
            return (
                <div className="flex size-8 items-center justify-center rounded-xl bg-slate-400/15 text-slate-600 dark:text-slate-300 shrink-0" aria-label="2">
                    <Medal className="size-4" aria-hidden="true" />
                </div>
            );
        }
        if (rank === 3) {
            return (
                <div className="flex size-8 items-center justify-center rounded-xl bg-orange-500/15 text-orange-700 dark:text-orange-400 shrink-0" aria-label="3">
                    <Medal className="size-4" aria-hidden="true" />
                </div>
            );
        }
        return (
            <div className="flex size-8 items-center justify-center rounded-xl text-sm font-semibold text-muted-foreground shrink-0 tabular-nums">
                {rank}
            </div>
        );
    };

    const renderList = (channels: ChannelData[], mode: RankSortMode) => {
        if (isLoading && !channelData) {
            return (
                <div className="flex items-center justify-center py-8">
                    <Loader2 className="size-8 animate-spin motion-reduce:animate-none text-muted-foreground" role="status" aria-label={tHome('feedback.loading')} />
                </div>
            );
        }
        if (isError && !channelData) {
            return (
                <div role="alert" className="flex flex-col items-center justify-center gap-3 py-8 text-sm text-muted-foreground">
                    <p>{tHome('feedback.loadFailed')}</p>
                    <InlineRetry onRetry={() => void refetch()} label={tHome('feedback.retry')} />
                </div>
            );
        }
        if (channels.length === 0) {
            return (
                <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
                    <TrendingUp className="size-10 opacity-40" aria-hidden="true" />
                    <p className="text-sm">{t('noData')}</p>
                </div>
            );
        }
        return (
            <div className="scrollbar space-y-3 max-h-[300px] overflow-y-auto">
                {channels.map((channel, index) => {
                    const rank = index + 1;

                    return (
                        <div
                            key={channel.raw.id}
                            className="flex items-center gap-3 p-3 rounded-2xl"
                        >
                            {getRankBadge(rank)}

                            <div className="flex-1 min-w-0">
                                <p className="font-medium text-sm truncate">{channel.raw.name}</p>
                                {mode === 'count' && (() => {
                                    const successCount = channel.formatted.request_success.raw;
                                    const failedCount = channel.formatted.request_failed.raw;
                                    const totalCount = successCount + failedCount;
                                    const successRate = totalCount > 0 ? (successCount / totalCount) * 100 : 0;

                                    return (
                                        <div className="flex items-center gap-1 text-xs text-muted-foreground mt-1">
                                            <span>{t('successRate')}:</span>
                                            <span>{successRate.toFixed(1)}%</span>
                                        </div>
                                    );
                                })()}
                            </div>

                            <div className="flex items-center gap-1 text-right shrink-0">
                                {mode === 'count' ? (
                                    <div className="flex items-center gap-1 text-sm font-medium tabular-nums">
                                        <span className="text-emerald-600 dark:text-emerald-400">
                                            {channel.formatted.request_success.formatted.value}
                                            <span className="text-xs text-muted-foreground">
                                                {channel.formatted.request_success.formatted.unit}
                                            </span>
                                        </span>
                                        <span className="text-muted-foreground/40 font-light">/</span>
                                        <span className="text-destructive">
                                            {channel.formatted.request_failed.formatted.value}
                                            <span className="text-xs text-muted-foreground">
                                                {channel.formatted.request_failed.formatted.unit}
                                            </span>
                                        </span>
                                    </div>
                                ) : mode === 'tokens' ? (
                                    <span className="font-semibold text-base">
                                        {channel.formatted.total_token.formatted.value}
                                        <span className="text-xs text-muted-foreground">
                                            {channel.formatted.total_token.formatted.unit}
                                        </span>
                                    </span>
                                ) : (
                                    <span className="font-semibold text-base">
                                        {channel.formatted.total_cost.formatted.value}
                                        <span className="text-xs text-muted-foreground">
                                            {channel.formatted.total_cost.formatted.unit}
                                        </span>
                                    </span>
                                )}
                            </div>
                        </div>
                    );
                })}
            </div>
        );
    };

    return (
        <div className="rounded-3xl bg-card text-card-foreground border-border border p-4">
            <Tabs value={rankSortMode} onValueChange={(value) => setRankSortMode(value as RankSortMode)}>
                <div className="flex items-center justify-between">
                    <h3 className="font-semibold text-base">{t('title')}</h3>
                    <TabsList>
                        <TabsTrigger value="cost">{t('sortByCost')}</TabsTrigger>
                        <TabsTrigger value="count">{t('sortByCount')}</TabsTrigger>
                        <TabsTrigger value="tokens">{t('sortByTokens')}</TabsTrigger>
                    </TabsList>
                </div>
                {isError && channelData && (
                    <div role="alert" className="mt-3 flex items-center justify-between gap-3 rounded-2xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-muted-foreground">
                        <span>{tHome('feedback.loadFailed')}</span>
                        <InlineRetry onRetry={() => void refetch()} label={tHome('feedback.retry')} className="shrink-0" />
                    </div>
                )}
                <TabsContents>
                    <TabsContent value="cost">
                        {renderList(rankedByCost, 'cost')}
                    </TabsContent>
                    <TabsContent value="count">
                        {renderList(rankedByCount, 'count')}
                    </TabsContent>
                    <TabsContent value="tokens">
                        {renderList(rankedByTokens, 'tokens')}
                    </TabsContent>
                </TabsContents>
            </Tabs>
        </div>
    );
}
