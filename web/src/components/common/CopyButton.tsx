import { useCallback, useEffect, useRef, useState } from 'react';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import { Check, Copy } from 'lucide-react';
import { useTranslations } from 'use-intl';
import { cn } from '@/lib/utils';
import { copyText } from '@/lib/clipboard';
import { toast } from '@/components/common/Toast';


export type CopyIconButtonProps = {
    text: string;
    className?: string;
    copyIconClassName?: string;
    checkIconClassName?: string;
};

export function CopyIconButton({
    text,
    className,
    copyIconClassName,
    checkIconClassName,
}: CopyIconButtonProps) {
    const t = useTranslations('common.copy');
    const [copied, setCopied] = useState(false);
    const timerRef = useRef<number | null>(null);
    const shouldReduceMotion = useReducedMotion() ?? false;

    useEffect(() => {
        return () => {
            if (timerRef.current) window.clearTimeout(timerRef.current);
        };
    }, []);

    const handleClick = useCallback(async () => {
        if (!text) {
            toast.error(t('failed'), { description: t('noContent') });
            return;
        }

        try {
            await copyText(text);

            setCopied(true);
            toast.success(t('success'));

            if (timerRef.current) window.clearTimeout(timerRef.current);
            timerRef.current = window.setTimeout(() => setCopied(false), 2000);
        } catch (err) {
            // 复制确实失败：清掉上一次的成功勾选，避免与失败提示并存。
            if (timerRef.current) {
                window.clearTimeout(timerRef.current);
                timerRef.current = null;
            }
            setCopied(false);
            const description = err instanceof Error ? err.message : String(err);
            toast.error(t('failed'), { description });
        }
    }, [
        text,
        t,
    ]);

    return (
        <button
            type="button"
            onClick={handleClick}
            aria-label={t('copy')}
            className={cn("focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring", className)}
        >
            <AnimatePresence mode="wait" initial={false}>
                {copied ? (
                    <motion.div
                        key="check"
                        initial={{ opacity: 0, scale: shouldReduceMotion ? 1 : 0.96 }}
                        animate={{ opacity: 1, scale: 1 }}
                        exit={{ opacity: 0, scale: shouldReduceMotion ? 1 : 0.96 }}
                    >
                        <Check className={cn(checkIconClassName)} />
                    </motion.div>
                ) : (
                    <motion.div
                        key="copy"
                        initial={{ opacity: 0, scale: shouldReduceMotion ? 1 : 0.96 }}
                        animate={{ opacity: 1, scale: 1 }}
                        exit={{ opacity: 0, scale: shouldReduceMotion ? 1 : 0.96 }}
                    >
                        <Copy className={cn(copyIconClassName)} />
                    </motion.div>
                )}
            </AnimatePresence>
        </button>
    );
}

