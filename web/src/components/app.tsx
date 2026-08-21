
import { useState, useEffect, useRef } from 'react';
import { motion, AnimatePresence, useReducedMotion } from "motion/react"
import { useAuth } from '@/api/user';
import { LoginForm } from '@/components/modules/login';
import { APIKeyDashboard } from '@/components/modules/apikey-dashboard';
import { ContentLoader } from '@/route/content-loader';
import { useNavStore } from '@/components/modules/navbar';
import Logo, { LOGO_DRAW_END_MS } from '@/components/modules/logo';
import { Toolbar } from '@/components/modules/toolbar';
import { AppShell } from '@/components/app-shell';
import {
    ENTRANCE_VARIANTS,
    REDUCED_MOTION_ENTRANCE_VARIANTS,
    REDUCED_MOTION_TRANSITION,
} from '@/lib/animations/fluid-transitions';
import {
    apiKeyDashboardStatsQueryOptions,
    apiKeyListQueryOptions,
    channelListQueryOptions,
    groupListQueryOptions,
    modelChannelListQueryOptions,
    modelListQueryOptions,
    statsDailyQueryOptions,
    statsHourlyQueryOptions,
    statsTotalQueryOptions,
} from '@/api/queries';
import { useQueryClient } from '@tanstack/react-query';
import { CONTENT_MAP } from '@/route';
import { logger } from '@/lib/logger';

function timeout(ms: number) {
    return new Promise<void>((resolve) => setTimeout(resolve, ms));
}

export function AppContainer() {
    const { isAuthenticated, isAPIKeyAuth, isLoading: authLoading } = useAuth();
    const { activeItem } = useNavStore();
    const queryClient = useQueryClient();
    const shouldReduceMotion = useReducedMotion() ?? false;
    const entranceVariants = shouldReduceMotion ? REDUCED_MOTION_ENTRANCE_VARIANTS : ENTRANCE_VARIANTS;

    // Logo 动画完成状态
    const [logoAnimationComplete, setLogoAnimationComplete] = useState(false);
    const [bootstrapComplete, setBootstrapComplete] = useState(false);
    const bootstrapStartedRef = useRef(false);

    // 首屏最早的 server-rendered loader：一旦客户端开始渲染，就淡出移除
    useEffect(() => {
        const el = document.getElementById('initial-loader');
        if (!el) return;

        el.classList.add('octo-hide');
        const timer = setTimeout(() => el.remove(), 220);
        return () => clearTimeout(timer);
    }, []);

    useEffect(() => {
        const timer = setTimeout(() => setLogoAnimationComplete(true), LOGO_DRAW_END_MS);
        return () => clearTimeout(timer);
    }, []);

    useEffect(() => {
        if (authLoading) return;
        if (!isAuthenticated) return;

        if (bootstrapStartedRef.current) return;
        bootstrapStartedRef.current = true;

        let cancelled = false;

        (async () => {
            try {
                const prefetches: Array<Promise<unknown>> = [];

                // API Key 认证模式：预取 dashboard stats
                if (isAPIKeyAuth) {
                    prefetches.push(queryClient.prefetchQuery(apiKeyDashboardStatsQueryOptions));
                } else {
                    // 普通用户认证模式：预取对应页面数据
                    const component = CONTENT_MAP[activeItem];
                    if (component?.preload) {
                        prefetches.push(component.preload());
                    }

                    switch (activeItem) {
                        case 'home': {
                            prefetches.push(
                                queryClient.prefetchQuery(statsTotalQueryOptions),
                                queryClient.prefetchQuery(statsDailyQueryOptions),
                                queryClient.prefetchQuery(statsHourlyQueryOptions),
                                queryClient.prefetchQuery(channelListQueryOptions),
                            );
                            break;
                        }
                        case 'channel': {
                            prefetches.push(queryClient.prefetchQuery(channelListQueryOptions));
                            break;
                        }
                        case 'group': {
                            prefetches.push(
                                queryClient.prefetchQuery(groupListQueryOptions),
                                queryClient.prefetchQuery(modelChannelListQueryOptions),
                            );
                            break;
                        }
                        case 'model': {
                            prefetches.push(queryClient.prefetchQuery(modelListQueryOptions));
                            break;
                        }
                        case 'setting': {
                            prefetches.push(queryClient.prefetchQuery(apiKeyListQueryOptions));
                            break;
                        }
                        default:
                            break;
                    }
                }

                await Promise.race([
                    Promise.allSettled(prefetches),
                    timeout(5000),
                ]);
            } catch (e) {
                logger.warn('bootstrap prefetch failed:', e);
            } finally {
                if (!cancelled) setBootstrapComplete(true);
            }
        })();

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [authLoading, isAuthenticated]);

    // 加载状态
    const isLoading =
        authLoading ||
        !logoAnimationComplete ||
        (isAuthenticated && !bootstrapComplete);

    // 加载页面
    if (isLoading) {
        return (
            <div className="min-h-screen flex items-center justify-center bg-background">
                <Logo size={120} animate />
            </div>
        );
    }

    // API Key 认证模式 - 显示 API Key Dashboard
    if (isAPIKeyAuth) {
        return (
            <AnimatePresence mode="wait">
                <APIKeyDashboard key="apikey-dashboard" />
            </AnimatePresence>
        );
    }

    // 登录页面
    if (!isAuthenticated) {
        return (
            <AnimatePresence mode="wait">
                <LoginForm key="login" />
            </AnimatePresence>
        );
    }

    // 主界面
    return (
        <AppShell actions={<Toolbar />}>
            <AnimatePresence mode="wait" initial={false}>
                <motion.div
                    key={activeItem}
                    variants={entranceVariants.content}
                    initial="initial"
                    animate="animate"
                    exit={shouldReduceMotion ? { opacity: 0 } : { opacity: 0, scale: 0.98 }}
                    transition={shouldReduceMotion ? REDUCED_MOTION_TRANSITION : { duration: 0.25 }}
                    className="h-full min-h-0 flex-1"
                >
                    <ContentLoader activeRoute={activeItem} />
                </motion.div>
            </AnimatePresence>
        </AppShell>
    );
}
