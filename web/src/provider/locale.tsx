import { useEffect, useState, type ReactNode } from 'react';
import { IntlProvider } from 'use-intl';
import { useSettingStore, type Locale } from '@/stores/setting';

import zh_hansMessages from '@/locales/zh_hans.json';
import zh_hantMessages from '@/locales/zh_hant.json';
import enMessages from '@/locales/en.json';

const messages: Record<Locale, typeof zh_hansMessages> = { // 各语言对应的客户端消息集合。
    zh_hans: zh_hansMessages,
    zh_hant: zh_hantMessages,
    en: enMessages,
};

// use-intl 需要合法的 BCP-47 locale tag（zh_hans 不是合法 tag，
// 会导致含 ICU 占位符的消息在 IntlMessageFormat 编译时抛错并回退为 key 文本）。
const intlLocales: Record<Locale, string> = {
    zh_hans: 'zh-Hans',
    zh_hant: 'zh-Hant',
    en: 'en',
};

export function LocaleProvider({ children }: { children: ReactNode }) {
    const { locale } = useSettingStore();
    const [currentLocale, setCurrentLocale] = useState<Locale>('zh_hans');

    useEffect(() => {
        setCurrentLocale(locale);
    }, [locale]);

    return (
        <IntlProvider
            locale={intlLocales[currentLocale]}
            messages={messages[currentLocale]}
            timeZone="Asia/Shanghai"
        >
            {children}
        </IntlProvider>
    );
}
