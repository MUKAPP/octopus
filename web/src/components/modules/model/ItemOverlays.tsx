import { useCallback, useEffect, useRef } from 'react';
import { Check, Loader2, Trash2, X } from 'lucide-react';
import { motion } from 'motion/react';
import { useTranslations } from 'use-intl';
import { Input } from '@/components/ui/input';
import { SPRING } from '@/lib/animations/fluid-transitions';
import { PriceScheduleEditor } from './PriceScheduleEditor';
import type { PriceScheduleDraft } from './price-schedule';

type EditValues = {
    input: string;
    output: string;
    cache_read: string;
    cache_write: string;
};

type ModelDeleteOverlayProps = {
    layoutId: string;
    isPending: boolean;
    onCancel: () => void;
    onConfirm: () => void;
};

export function ModelDeleteOverlay({
    layoutId,
    isPending,
    onCancel,
    onConfirm,
}: ModelDeleteOverlayProps) {
    const t = useTranslations('model.overlay');
    return (
        <motion.div
            layoutId={layoutId}
            className="absolute inset-0 flex items-center justify-center gap-3 bg-destructive p-4 rounded-3xl"
            transition={SPRING.morph}
        >
            <button
                type="button"
                onClick={onCancel}
                className="focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring h-9 px-4 flex items-center justify-center gap-1.5 rounded-xl bg-destructive-foreground/20 text-destructive-foreground text-sm font-medium transition-[color,background-color,transform] motion-reduce:transition-none hover:bg-destructive-foreground/30 active:scale-[0.98] motion-reduce:transform-none"
            >
                <X className="size-4" />
                {t('cancel')}
            </button>
            <button
                type="button"
                onClick={onConfirm}
                disabled={isPending}
                className="focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring h-9 px-4 flex items-center justify-center gap-1.5 rounded-xl bg-destructive-foreground text-destructive text-sm font-medium transition-[color,background-color,transform] motion-reduce:transition-none hover:bg-destructive-foreground/90 active:scale-[0.98] motion-reduce:transform-none disabled:opacity-50 disabled:cursor-not-allowed"
            >
                {isPending ? (
                    <Loader2 className="size-4 animate-spin motion-reduce:animate-none" />
                ) : (
                    <Trash2 className="size-4" />
                )}
                {isPending ? t('deleting') : t('confirmDelete')}
            </button>
        </motion.div>
    );
}

type ModelEditOverlayProps = {
    layoutId: string;
    onHeightChange?: (height: number) => void;
    modelName: string;
    editValues: EditValues;
    schedule: PriceScheduleDraft | null;
    onScheduleChange: (value: PriceScheduleDraft | null) => void;
    isPending: boolean;
    onChange: (next: EditValues) => void;
    onCancel: () => void;
    onSave: () => void;
};

export function ModelEditOverlay({
    layoutId,
    onHeightChange,
    modelName,
    editValues,
    schedule,
    onScheduleChange,
    isPending,
    onChange,
    onCancel,
    onSave,
}: ModelEditOverlayProps) {
    const t = useTranslations('model.overlay');
    const overlayRef = useRef<HTMLDivElement | null>(null);
    const setOverlayRef = useCallback((node: HTMLDivElement | null) => {
        overlayRef.current = node;
        if (node) onHeightChange?.(node.offsetHeight);
    }, [onHeightChange]);

    useEffect(() => {
        const node = overlayRef.current;
        if (!node || !onHeightChange || typeof ResizeObserver === 'undefined') return;

        const observer = new ResizeObserver(() => onHeightChange(node.offsetHeight));
        observer.observe(node);
        return () => observer.disconnect();
    }, [onHeightChange]);

    return (
        <motion.div
            ref={setOverlayRef}
            layoutId={layoutId}
            className="z-20 flex flex-col bg-card p-5 rounded-3xl border border-border scrollbar overflow-y-auto max-h-[calc(100dvh-var(--safe-area-top)-var(--safe-area-bottom))]"
            transition={SPRING.morph}
        >
            <form onSubmit={event => { event.preventDefault(); onSave(); }} className="min-w-0">
            <h3 className="text-sm font-semibold text-card-foreground line-clamp-1 mb-3">
                {modelName}
            </h3>

            <div className="grid grid-cols-2 gap-2">
                <label className="grid gap-1 text-xs text-muted-foreground">
                    {t('input')}
                    <Input
                        type="number"
                        step="any"
                        min="0"
                        value={editValues.input}
                        onChange={(e) => onChange({ ...editValues, input: e.target.value })}
                        className="h-9 text-base sm:text-sm rounded-xl"
                    />
                </label>
                <label className="grid gap-1 text-xs text-muted-foreground">
                    {t('output')}
                    <Input
                        type="number"
                        step="any"
                        min="0"
                        value={editValues.output}
                        onChange={(e) => onChange({ ...editValues, output: e.target.value })}
                        className="h-9 text-base sm:text-sm rounded-xl"
                    />
                </label>
                <label className="grid gap-1 text-xs text-muted-foreground">
                    {t('cacheRead')}
                    <Input
                        type="number"
                        step="any"
                        min="0"
                        value={editValues.cache_read}
                        onChange={(e) => onChange({ ...editValues, cache_read: e.target.value })}
                        className="h-9 text-base sm:text-sm rounded-xl"
                    />
                </label>
                <label className="grid gap-1 text-xs text-muted-foreground">
                    {t('cacheWrite')}
                    <Input
                        type="number"
                        step="any"
                        min="0"
                        value={editValues.cache_write}
                        onChange={(e) => onChange({ ...editValues, cache_write: e.target.value })}
                        className="h-9 text-base sm:text-sm rounded-xl"
                    />
                </label>
            </div>

            <div className="mt-4">
                <PriceScheduleEditor value={schedule} onChange={onScheduleChange} basePrice={{
                    input: Number(editValues.input) || 0, output: Number(editValues.output) || 0,
                    cache_read: Number(editValues.cache_read) || 0, cache_write: Number(editValues.cache_write) || 0,
                }} />
            </div>
            <div className="flex gap-2 pt-2 mt-3">
                <button
                    type="button"
                    onClick={onCancel}
                    disabled={isPending}
                    className="focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring flex-1 h-9 flex items-center justify-center gap-1.5 rounded-xl bg-muted text-muted-foreground text-sm font-medium transition-[color,background-color,transform] motion-reduce:transition-none hover:bg-muted/80 active:scale-[0.98] motion-reduce:transform-none disabled:opacity-50"
                >
                    <X className="size-4" />
                    {t('cancel')}
                </button>
                <button
                    type="submit"
                    disabled={isPending}
                    className="focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring flex-1 h-9 flex items-center justify-center gap-1.5 rounded-xl bg-primary text-primary-foreground text-sm font-medium transition-[color,background-color,transform] motion-reduce:transition-none hover:bg-primary/90 active:scale-[0.98] motion-reduce:transform-none disabled:opacity-50"
                >
                    {isPending ? <Loader2 className="size-4 animate-spin motion-reduce:animate-none" /> : <Check className="size-4" />}
                    {t('save')}
                </button>
            </div>
            </form>
        </motion.div>
    );
}
