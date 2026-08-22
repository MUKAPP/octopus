import { queryOptions } from '@tanstack/react-query';
import type { APIKey, APIKeyStatsResponse } from './apikey';
import type { ChannelServer } from './channel';
import type { Group } from './group';
import type { LLMChannel, LLMInfo } from './model';
import type { StatsDaily, StatsHourly, StatsTotal } from './stats';
import { apiRequest } from './client';

// Shared query definitions keep shell preloading and page hooks on the same cache keys.
export const apiKeyDashboardStatsQueryOptions = queryOptions({
    queryKey: ['apikey', 'dashboard', 'stats'] as const,
    queryFn: () => apiRequest<APIKeyStatsResponse>('/api/v1/apikey/stats'),
});

export const apiKeyListQueryOptions = queryOptions({
    queryKey: ['apikeys', 'list'] as const,
    queryFn: () => apiRequest<APIKey[]>('/api/v1/apikey/list'),
});

export const channelListQueryOptions = queryOptions({
    queryKey: ['channels', 'list'] as const,
    queryFn: () => apiRequest<ChannelServer[]>('/api/v1/channel/list'),
});

export const groupListQueryOptions = queryOptions({
    queryKey: ['groups', 'list'] as const,
    queryFn: () => apiRequest<Group[]>('/api/v1/group/list'),
});

export const modelListQueryOptions = queryOptions({
    queryKey: ['models', 'list'] as const,
    queryFn: () => apiRequest<LLMInfo[]>('/api/v1/model/list'),
});

export const modelChannelListQueryOptions = queryOptions({
    queryKey: ['models', 'channel'] as const,
    queryFn: () => apiRequest<LLMChannel[]>('/api/v1/model/channel'),
});

export const statsDailyQueryOptions = queryOptions({
    queryKey: ['stats', 'daily'] as const,
    queryFn: () => apiRequest<StatsDaily[]>('/api/v1/stats/daily'),
});

export const statsHourlyQueryOptions = queryOptions({
    queryKey: ['stats', 'hourly'] as const,
    queryFn: () => apiRequest<StatsHourly[]>('/api/v1/stats/hourly'),
});

export const statsTotalQueryOptions = queryOptions({
    queryKey: ['stats', 'total'] as const,
    queryFn: () => apiRequest<StatsTotal>('/api/v1/stats/total'),
});
