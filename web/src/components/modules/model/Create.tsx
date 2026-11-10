import { useState } from 'react';
import { useCreateModel } from '@/api/model';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Field, FieldLabel, FieldGroup } from '@/components/ui/field';
import {
    MorphingDialogClose,
    MorphingDialogTitle,
    MorphingDialogDescription,
    useMorphingDialog,
} from '@/components/ui/morphing-dialog';
import { useTranslations } from 'use-intl';
import { toast } from '@/components/common/Toast';
import { PriceScheduleEditor } from './PriceScheduleEditor';
import { fromScheduleDraft, type PriceScheduleDraft } from './price-schedule';

export function CreateDialogContent() {
    const { setIsOpen } = useMorphingDialog();
    const t = useTranslations('model.create');
    const tToast = useTranslations('model.toast');
    const createModel = useCreateModel();
    const [schedule, setSchedule] = useState<PriceScheduleDraft | null>(null);

    const [formData, setFormData] = useState({
        name: '',
        input: '',
        output: '',
        cache_read: '',
        cache_write: '',
    });

    const handleSubmit = (event: React.FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        if (!formData.name.trim()) return;

        createModel.mutate({
            name: formData.name.trim(),
            price_schedule: fromScheduleDraft(schedule),
            input: parseFloat(formData.input) || 0,
            output: parseFloat(formData.output) || 0,
            cache_read: parseFloat(formData.cache_read) || 0,
            cache_write: parseFloat(formData.cache_write) || 0,
        }, {
            onSuccess: () => {
                setFormData({ name: '', input: '', output: '', cache_read: '', cache_write: '' });
                setSchedule(null);
                setIsOpen(false);
            },
            onError: (error) => {
                toast.error(tToast('createFailed'), { description: error.message });
            }
        });
    };

    return (
        <>
            <MorphingDialogTitle className="shrink-0">
                <header className="mb-4 flex items-center justify-between gap-3">
                    <h2 className="text-2xl font-bold text-card-foreground">{t('title')}</h2>
                    <MorphingDialogClose
                        className="relative right-0 top-0 shrink-0"
                        variants={{
                            initial: { opacity: 0, scale: 0.8 },
                            animate: { opacity: 1, scale: 1 },
                            exit: { opacity: 0, scale: 0.8 },
                        }}
                    />
                </header>
            </MorphingDialogTitle>
            <form onSubmit={handleSubmit} className="flex min-h-0 min-w-0 flex-1 flex-col">
                <MorphingDialogDescription className="scrollbar -mx-1 flex-1 min-h-0 min-w-0 overflow-y-auto overscroll-contain px-1 py-1">
                    <FieldGroup className="gap-4">
                        <Field>
                            <FieldLabel htmlFor="model-name">{t('name')}</FieldLabel>
                            <Input
                                id="model-name"
                                value={formData.name}
                                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                                className="rounded-xl text-base sm:text-sm"
                                required
                            />
                        </Field>
                        <div className="grid grid-cols-1 gap-3 min-[360px]:grid-cols-2 sm:grid-cols-4">
                            <Field>
                                <FieldLabel htmlFor="model-input">{t('input')}</FieldLabel>
                                <Input
                                    id="model-input"
                                    type="number"
                                    step="any"
                                    min="0"
                                    value={formData.input}
                                    onChange={(e) => setFormData({ ...formData, input: e.target.value })}
                                    className="rounded-xl tabular-nums text-base sm:text-sm"
                                />
                            </Field>
                            <Field>
                                <FieldLabel htmlFor="model-output">{t('output')}</FieldLabel>
                                <Input
                                    id="model-output"
                                    type="number"
                                    step="any"
                                    min="0"
                                    value={formData.output}
                                    onChange={(e) => setFormData({ ...formData, output: e.target.value })}
                                    className="rounded-xl tabular-nums text-base sm:text-sm"
                                />
                            </Field>
                            <Field>
                                <FieldLabel htmlFor="model-cache-read">{t('cacheRead')}</FieldLabel>
                                <Input
                                    id="model-cache-read"
                                    type="number"
                                    step="any"
                                    min="0"
                                    value={formData.cache_read}
                                    onChange={(e) => setFormData({ ...formData, cache_read: e.target.value })}
                                    className="rounded-xl tabular-nums text-base sm:text-sm"
                                />
                            </Field>
                            <Field>
                                <FieldLabel htmlFor="model-cache-write">{t('cacheWrite')}</FieldLabel>
                                <Input
                                    id="model-cache-write"
                                    type="number"
                                    step="any"
                                    min="0"
                                    value={formData.cache_write}
                                    onChange={(e) => setFormData({ ...formData, cache_write: e.target.value })}
                                    className="rounded-xl tabular-nums text-base sm:text-sm"
                                />
                            </Field>
                        </div>
                        <p className="text-xs text-muted-foreground">{t('priceHint')}</p>
                        <PriceScheduleEditor value={schedule} onChange={setSchedule} basePrice={{
                            input: Number(formData.input) || 0, output: Number(formData.output) || 0,
                            cache_read: Number(formData.cache_read) || 0, cache_write: Number(formData.cache_write) || 0,
                        }} />
                    </FieldGroup>
                </MorphingDialogDescription>
                <div className="mt-3 flex shrink-0 gap-2 border-t border-border/50 pt-4">
                    <Button
                        type="button"
                        variant="secondary"
                        disabled={createModel.isPending}
                        className="h-11 flex-1 rounded-xl"
                        onClick={() => setIsOpen(false)}
                    >
                        {t('cancel')}
                    </Button>
                    <Button
                        type="submit"
                        disabled={createModel.isPending || !formData.name.trim()}
                        className="h-11 flex-1 rounded-xl"
                    >
                        {createModel.isPending ? t('submitting') : t('submit')}
                    </Button>
                </div>
            </form>
        </>
    );
}
