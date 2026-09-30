import { Activity } from './activity';
import { Total } from './total';
import { StatsChart } from './chart';
import { Rank } from './rank';
import { PageWrapper } from '@/components/common/PageWrapper';

export function Home() {
    return (
        <PageWrapper className="scrollbar h-full min-h-0 overflow-y-auto overscroll-contain space-y-6 pb-nav-clearance px-2 md:pb-[calc(1rem+var(--safe-area-bottom))] rounded-t-3xl">
            <Total />
            <Activity />
            <StatsChart />
            <Rank />
        </PageWrapper>
    );
}
