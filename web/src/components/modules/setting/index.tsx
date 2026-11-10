import { PageWrapper } from '@/components/common/PageWrapper';
import { SettingAppearance } from './Appearance';
import { SettingSystem } from './System';
import { SettingAPIKey } from './APIKey';
import { SettingLLMPrice } from './LLMPrice';
import { SettingAccount } from './Account';
import { SettingInfo } from './Info';
import { SettingLLMSync } from './LLMSync';
import { SettingLog } from './Log';
import { SettingBackup } from './Backup';
import { SettingCircuitBreaker } from './CircuitBreaker';

export function Setting() {
    return (
        <div className="scrollbar h-full min-h-0 overflow-y-auto overscroll-contain rounded-t-3xl">
            <PageWrapper className="grid grid-cols-1 gap-4 pb-nav-clearance md:grid-cols-2 md:items-start md:pb-[calc(1rem+var(--safe-area-bottom))]">
                <div className="flex min-w-0 flex-col gap-4">
                    <SettingInfo />
                    <SettingAppearance />
                    <SettingAccount />
                    <SettingSystem />
                    <SettingCircuitBreaker />
                </div>
                <div className="flex min-w-0 flex-col gap-4">
                    <SettingAPIKey />
                    <SettingLLMPrice />
                    <SettingLLMSync />
                    <SettingLog />
                    <SettingBackup />
                </div>
            </PageWrapper>
        </div>
    );
}
