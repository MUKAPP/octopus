import { useEffect } from 'react';
import { useMutation } from '@tanstack/react-query';
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { apiRequest, apiUnauthorizedEvent, setAPIKey } from './client';
import { logger } from '@/lib/logger';

/**
 * 用户登录请求
 */
export interface UserLoginRequest {
    username: string;
    password: string;
    expire: number; // 登录状态过期时间，正数为秒，-1 表示 30 天。
}

/**
 * 认证状态 Store
 */
interface AuthState {
    isAuthenticated: boolean;
    isLoading: boolean;
    isAPIKeyAuth: boolean;
    token: string | null;

    setAuth: () => void;
    setAPIKeyAuth: (apiKey: string) => void;
    checkAuth: () => Promise<void>;
    logout: () => void;
}

/**
 * 认证状态管理 Store（使用 zustand + persist）
 */
export const useAuthStore = create<AuthState>()(
    persist(
        (set, get) => ({
            isAuthenticated: false,
            isLoading: true,
            isAPIKeyAuth: false,
            token: null,

            setAuth: () => {
                setAPIKey(null);
                set({
                    isAuthenticated: true,
                    isAPIKeyAuth: false,
                    token: null,
                    isLoading: false,
                });
            },

            setAPIKeyAuth: (apiKey: string) => {
                setAPIKey(apiKey);
                set({
                    isAuthenticated: true,
                    isAPIKeyAuth: true,
                    token: apiKey,
                    isLoading: false,
                });
            },

            checkAuth: async () => {
                const { token, isAPIKeyAuth } = get();
                setAPIKey(isAPIKeyAuth ? token : null);

                // Ordinary users authenticate through the HttpOnly cookie, so they
                // must still probe /user/status when no persisted token exists.
                if (isAPIKeyAuth && !token) {
                    set({ isAuthenticated: false, isLoading: false });
                    return;
                }

                try {
                    const endpoint = isAPIKeyAuth ? '/api/v1/apikey/login' : '/api/v1/user/status';
                    await apiRequest<unknown>(endpoint, { dispatchUnauthorized: false });
                    set({
                        isAuthenticated: true,
                        isLoading: false,
                        token: isAPIKeyAuth ? token : null,
                    });
                } catch (error) {
                    logger.error('认证验证失败:', error);
                    get().logout();
                }
            },

            logout: () => {
                setAPIKey(null);
                set({
                    isAuthenticated: false,
                    isAPIKeyAuth: false,
                    token: null,
                    isLoading: false,
                });
                void apiRequest('/api/v1/user/logout', {
                    method: 'POST',
                    dispatchUnauthorized: false,
                }).catch(() => undefined);
            },
        }),
        {
            name: 'auth-storage',
            partialize: (state) => ({
                token: state.isAPIKeyAuth ? state.token : null,
                isAPIKeyAuth: state.isAPIKeyAuth,
            }),
        }
    )
);

/**
 * 用户登录 Hook
 */
export function useLogin() {
    const { setAuth } = useAuthStore();

    return useMutation({
        mutationFn: async (data: UserLoginRequest) => {
            setAPIKey(null);
            return apiRequest<string>('/api/v1/user/login', {
                method: 'POST',
                body: data,
                dispatchUnauthorized: false,
            });
        },
        onSuccess: () => {
            setAuth();
        },
        onError: (error) => {
            logger.error('登录失败:', error);
        },
    });
}

/**
 * 修改密码 Hook
 */
export function useChangePassword() {
    return useMutation({
        mutationFn: (data: { oldPassword: string; newPassword: string }) =>
            apiRequest<string>('/api/v1/user/change-password', {
                method: 'POST',
                body: {
                    old_password: data.oldPassword,
                    new_password: data.newPassword,
                },
            }),
        onSuccess: (message) => {
            logger.log('密码修改成功:', message);
        },
        onError: (error) => {
            logger.error('密码修改失败:', error);
        },
    });
}

/**
 * 修改用户名 Hook
 */
export function useChangeUsername() {
    return useMutation({
        mutationFn: (data: { newUsername: string }) =>
            apiRequest<string>('/api/v1/user/change-username', {
                method: 'POST',
                body: { new_username: data.newUsername },
            }),
        onSuccess: (message) => {
            logger.log('用户名修改成功:', message);
        },
        onError: (error) => {
            logger.error('用户名修改失败:', error);
        },
    });
}

/**
 * 认证状态和方法 Hook
 */
export function useAuth() {
    const store = useAuthStore();
    const { checkAuth, isLoading } = store;

    useEffect(() => {
        const handleUnauthorized = () => useAuthStore.getState().logout();
        window.addEventListener(apiUnauthorizedEvent, handleUnauthorized);
        if (isLoading) {
            void checkAuth();
        }
        return () => window.removeEventListener(apiUnauthorizedEvent, handleUnauthorized);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    return {
        isAuthenticated: store.isAuthenticated,
        isAPIKeyAuth: store.isAPIKeyAuth,
        isLoading: store.isLoading,
        logout: store.logout,
    };
}

