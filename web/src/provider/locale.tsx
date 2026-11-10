import { useEffect, type ReactNode } from 'react';
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

// 同步 <html lang>：索引页固定写死 zh-CN，切换语言后若不同步，
// 屏幕阅读器发音与浏览器翻译都会按中文处理。
const documentLangs: Record<Locale, string> = {
    zh_hans: 'zh-CN',
    zh_hant: 'zh-Hant',
    en: 'en',
};

export function LocaleProvider({ children }: { children: ReactNode }) {
    const { locale } = useSettingStore();

    useEffect(() => {
        document.documentElement.lang = documentLangs[locale];
    }, [locale]);

    return (
        <IntlProvider
            locale={intlLocales[locale]}
            messages={messages[locale]}
            timeZone="Asia/Shanghai"
        >
            {children}
        </IntlProvider>
    );
}
