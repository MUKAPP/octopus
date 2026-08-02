import { useMemo } from 'react';
import { SearchX } from 'lucide-react';
import { useTranslations } from 'use-intl';
import { useChannelList } from '@/api/endpoints/channel';
import { Card } from './Card';
import { useSearchStore, useToolbarViewOptionsStore } from '@/components/modules/toolbar';
import { VirtualizedGrid } from '@/components/common/VirtualizedGrid';

export function Channel() {
    const { data: channelsData, isLoading } = useChannelList();
    const t = useTranslations('channel');
    const pageKey = 'channel' as const;
    const searchTerm = useSearchStore((s) => s.getSearchTerm(pageKey));
    const layout = useToolbarViewOptionsStore((s) => s.getLayout(pageKey));
    const sortField = useToolbarViewOptionsStore((s) => s.getSortField(pageKey));
    const sortOrder = useToolbarViewOptionsStore((s) => s.getSortOrder(pageKey));
    const filter = useToolbarViewOptionsStore((s) => s.channelFilter);

    const sortedChannels = useMemo(() => {
        if (!channelsData) return [];
        return [...channelsData].sort((a, b) => {
            const diff = sortField === 'name'
                ? a.raw.name.localeCompare(b.raw.name)
                : a.raw.id - b.raw.id;
            return sortOrder === 'asc' ? diff : -diff;
        });
    }, [channelsData, sortField, sortOrder]);

    const visibleChannels = useMemo(() => {
        const term = searchTerm.toLowerCase().trim();
        const byName = !term ? sortedChannels : sortedChannels.filter((c) => c.raw.name.toLowerCase().includes(term));

        if (filter === 'enabled') return byName.filter((c) => c.raw.enabled);
        if (filter === 'disabled') return byName.filter((c) => !c.raw.enabled);

        return byName;
    }, [sortedChannels, searchTerm, filter]);

    return (
        <VirtualizedGrid
            items={visibleChannels}
            isLoading={isLoading}
            layout={layout}
            columns={{ default: 1, md: 2, lg: 3 }}
            estimateItemHeight={216}
            emptyState={
                <div className="flex flex-col items-center gap-3 text-center text-muted-foreground">
                    <SearchX className="size-10 opacity-40" aria-hidden="true" />
                    <p className="text-sm">{channelsData?.length ? t('noResults') : t('empty')}</p>
                </div>
            }
            getItemKey={(item) => `channel-${item.raw.id}`}
            renderItem={(item) => <Card channel={item.raw} stats={item.formatted} layout={layout} />}
        />
    );
}
