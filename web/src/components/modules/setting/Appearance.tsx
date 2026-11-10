import { useTheme } from '@/provider/theme';
import { useTranslations } from 'use-intl';
import { Sun, Moon, Monitor, Languages } from 'lucide-react';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { useSettingStore, type Locale } from '@/stores/setting';

export function SettingAppearance() {
    const t = useTranslations('setting');
    const { theme, resolvedTheme, setTheme } = useTheme();
    const { locale, setLocale } = useSettingStore();

    return (
        <div className="rounded-3xl border border-border bg-card p-6 space-y-5">
            <h2 className="text-lg font-bold text-card-foreground flex items-center gap-2">
                <Sun className="h-5 w-5" />
                {t('appearance')}
            </h2>

            {/* 主题 */}
            <div className="flex flex-wrap items-center justify-between gap-3 sm:gap-4">
                <div className="flex items-center gap-3">
                    {resolvedTheme === 'dark' ? <Moon className="h-5 w-5 text-muted-foreground" /> : <Sun className="h-5 w-5 text-muted-foreground" />}
                    <span className="text-sm font-medium">{t('theme.label')}</span>
                </div>
                <Select value={theme} onValueChange={setTheme}>
                    <SelectTrigger className="w-full min-[360px]:w-36" aria-label={t('theme.label')}>
                        <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                        <SelectItem value="light">
                            <Sun className="size-4" />
                            {t('theme.light')}
                        </SelectItem>
                        <SelectItem value="dark">
                            <Moon className="size-4" />
                            {t('theme.dark')}
                        </SelectItem>
                        <SelectItem value="system">
                            <Monitor className="size-4" />
                            {t('theme.system')}
                        </SelectItem>
                    </SelectContent>
                </Select>
            </div>

            {/* 语言 */}
            <div className="flex flex-wrap items-center justify-between gap-3 sm:gap-4">
                <div className="flex items-center gap-3">
                    <Languages className="h-5 w-5 text-muted-foreground" />
                    <span className="text-sm font-medium">{t('language.label')}</span>
                </div>
                <Select value={locale} onValueChange={(v) => setLocale(v as Locale)}>
                    <SelectTrigger className="w-full min-[360px]:w-36" aria-label={t('language.label')}>
                        <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                        <SelectItem value="zh_hans">{t('language.zh_hans')}</SelectItem>
                        <SelectItem value="zh_hant">{t('language.zh_hant')}</SelectItem>
                        <SelectItem value="en">{t('language.en')}</SelectItem>
                    </SelectContent>
                </Select>
            </div>
        </div>
    );
}
