import { useQuery } from '@tanstack/react-query';
import { apiClient } from '../client';

/** Docker Hub 中已发布的最新开发镜像版本。 */
export interface LatestInfo {
    tag_name: string;
    published_at: string;
}

/** 获取与当前 dev 分支提交对应、且已发布到 Docker Hub 的镜像版本。 */
export function useLatestInfo() {
    return useQuery({
        queryKey: ['update', 'latest'],
        queryFn: async () => apiClient.get<LatestInfo>('/api/v1/update'),
        refetchInterval: 3600000,
        refetchOnMount: 'always',
    });
}

/** 获取当前后端构建版本。 */
export function useNowVersion() {
    return useQuery({
        queryKey: ['update', 'now-version'],
        queryFn: async () => apiClient.get<string>('/api/v1/update/now-version'),
        refetchInterval: 3600000,
        refetchOnMount: 'always',
    });
}

