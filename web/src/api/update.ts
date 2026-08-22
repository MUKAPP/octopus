import { useQuery } from '@tanstack/react-query';
import { apiRequest } from './client';

/** 最新可用版本：Docker Hub latest 镜像与 GitHub Releases 二进制槽位中较新者。 */
export interface LatestInfo {
    tag_name: string;
    published_at: string;
}

/** 获取与当前部署匹配的最新版本（后端按版本形态选择 Docker Hub 或 GitHub Releases 源）。 */
export function useLatestInfo(current?: string) {
    return useQuery({
        queryKey: ['update', 'latest', current ?? ''],
        queryFn: async () => {
            const params: Record<string, string | number | boolean> | undefined = current
                ? { current }
                : undefined;
            return apiRequest<LatestInfo>('/api/v1/update', { params });
        },
        refetchInterval: 3600000,
        refetchOnMount: 'always',
        enabled: !!current,
    });
}

/** 获取当前后端构建版本。 */
export function useNowVersion() {
    return useQuery({
        queryKey: ['update', 'now-version'],
        queryFn: async () => apiRequest<string>('/api/v1/update/now-version'),
        refetchInterval: 3600000,
        refetchOnMount: 'always',
    });
}

