import type { ReactNode } from 'react';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import { useTranslations } from 'use-intl';
import Logo from '@/components/modules/logo';
import { NavBar, useNavStore } from '@/components/modules/navbar';
import {
    REDUCED_MOTION_ROUTE_TITLE_VARIANTS,
    REDUCED_MOTION_TRANSITION,
    ROUTE_TITLE_VARIANTS,
} from '@/lib/animations/fluid-transitions';

// AppShell 保持普通用户界面的稳定布局，统一渲染导航、顶栏和页面内容。
export function AppShell({ children, actions }: { children: ReactNode; actions?: ReactNode }) {
    const { activeItem, direction } = useNavStore();
    const t = useTranslations('navbar');
    const shouldReduceMotion = useReducedMotion() ?? false;
    const routeTitleVariants = shouldReduceMotion
        ? REDUCED_MOTION_ROUTE_TITLE_VARIANTS
        : ROUTE_TITLE_VARIANTS;

    return (
        <motion.div
            key="main-app"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ duration: 0.3 }}
            className="mx-auto flex h-dvh max-w-6xl flex-col overflow-hidden px-3 md:grid md:grid-cols-[auto_1fr] md:gap-6 md:px-6"
        >
            <NavBar />
            <main className="flex min-h-0 w-full min-w-0 flex-1 flex-col">
                <header className="my-6 flex flex-none items-center gap-x-2 px-2">
                    <Logo size={48} />
                    <div className="min-w-0 flex-1 overflow-hidden">
                        <AnimatePresence mode="wait" custom={direction}>
                            <motion.div
                                key={activeItem}
                                custom={direction}
                                variants={routeTitleVariants}
                                initial="initial"
                                animate="animate"
                                exit="exit"
                                transition={shouldReduceMotion ? REDUCED_MOTION_TRANSITION : { duration: 0.3 }}
                                className="flex items-center"
                            >
                                <span className="mt-1 truncate text-3xl font-bold">{t(activeItem)}</span>
                            </motion.div>
                        </AnimatePresence>
                    </div>
                    {actions && <div className="ml-auto">{actions}</div>}
                </header>
                {children}
            </main>
        </motion.div>
    );
}
