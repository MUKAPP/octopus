import type { ComponentType } from 'react';
import type { SvgIconProps } from '@thesvg/react';
import NewAPIIcon from '@thesvg/react/new-api';
import OpenAIIcon from '@thesvg/react/openai-chatgpt';
import AnthropicIcon from '@thesvg/react/anthropic';
import VolcengineIcon from '@thesvg/react/volcengine';
import DeepSeekIcon from '@thesvg/react/deepseek';
import OpenRouterIcon from '@thesvg/react/openrouter';
import GroqIcon from '@thesvg/react/groq';
import QwenIcon from '@thesvg/react/qwen';
import MoonshotIcon from '@thesvg/react/moonshot-ai';
import ZhipuIcon from '@thesvg/react/zhipu';
import XAIIcon from '@thesvg/react/xai-grok';
import SiliconFlowIcon from '@thesvg/react/siliconcloud-siliconflow';
import AzureIcon from '@thesvg/react/azure-azure-openai';
import { ChannelType } from '@/api/channel';

// ChannelPreset 是创建渠道时可选的服务商预填模板。
// 模板只预填协议类型与基础地址: 其名称与说明来自 channel.create.presets 的本地化消息,
// 主协议之外的其他协议由用户手动切换, 不做地址自动改写。
export type ChannelPreset = {
    id: string;
    type: ChannelType;
    baseUrl: string;
    Icon: ComponentType<SvgIconProps>;
    iconClassName?: string; // 单色图标在深色主题下需反色。
};

export const CHANNEL_PRESETS: ChannelPreset[] = [
    {
        id: 'newapi',
        type: ChannelType.OpenAIChat,
        baseUrl: '',
        Icon: NewAPIIcon,
    },
    {
        id: 'openai',
        type: ChannelType.OpenAIChat,
        baseUrl: 'https://api.openai.com',
        Icon: OpenAIIcon,
        iconClassName: 'brightness-0 dark:invert',
    },
    {
        id: 'anthropic',
        type: ChannelType.Anthropic,
        baseUrl: 'https://api.anthropic.com',
        Icon: AnthropicIcon,
        iconClassName: 'brightness-0 dark:invert',
    },
    {
        id: 'volcengine',
        type: ChannelType.Volcengine,
        baseUrl: 'https://ark.cn-beijing.volces.com/api/v3',
        Icon: VolcengineIcon,
    },
    {
        id: 'volcengine-coding-plan',
        type: ChannelType.Volcengine,
        baseUrl: 'https://ark.cn-beijing.volces.com/api/coding/v3',
        Icon: VolcengineIcon,
    },
    {
        id: 'deepseek',
        type: ChannelType.OpenAIChat,
        // 末尾 "#" 沿用 NormalizeLegacyBaseURL 的既有语义, 跳过默认的 /v1 版本补全。
        baseUrl: 'https://api.deepseek.com#',
        Icon: DeepSeekIcon,
    },
    {
        id: 'openrouter',
        type: ChannelType.OpenAIChat,
        baseUrl: 'https://openrouter.ai/api',
        Icon: OpenRouterIcon,
        iconClassName: 'brightness-0 dark:invert',
    },
    {
        id: 'groq',
        type: ChannelType.OpenAIChat,
        baseUrl: 'https://api.groq.com/openai',
        Icon: GroqIcon,
    },
    {
        id: 'dashscope',
        type: ChannelType.OpenAIChat,
        baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
        Icon: QwenIcon,
        iconClassName: 'brightness-0 dark:invert',
    },
    {
        id: 'qwen-token-plan',
        type: ChannelType.OpenAIChat,
        baseUrl: 'https://token-plan.maas.qianwenaiapi.com/compatible-mode/v1',
        Icon: QwenIcon,
        iconClassName: 'brightness-0 dark:invert',
    },
    {
        id: 'moonshot',
        type: ChannelType.OpenAIChat,
        baseUrl: 'https://api.moonshot.cn',
        Icon: MoonshotIcon,
        iconClassName: 'brightness-0 dark:invert',
    },
    {
        id: 'kimi-code',
        type: ChannelType.OpenAIChat,
        baseUrl: 'https://api.kimi.com/coding/v1',
        Icon: MoonshotIcon,
        iconClassName: 'brightness-0 dark:invert',
    },
    {
        id: 'zhipu',
        type: ChannelType.OpenAIChat,
        baseUrl: 'https://open.bigmodel.cn/api/paas/v4',
        Icon: ZhipuIcon,
    },
    {
        id: 'glm-coding-plan',
        type: ChannelType.OpenAIChat,
        baseUrl: 'https://open.bigmodel.cn/api/coding/paas/v4',
        Icon: ZhipuIcon,
    },
    {
        id: 'xai',
        type: ChannelType.OpenAIChat,
        baseUrl: 'https://api.x.ai',
        Icon: XAIIcon,
        iconClassName: 'brightness-0 dark:invert',
    },
    {
        id: 'siliconflow',
        type: ChannelType.OpenAIChat,
        baseUrl: 'https://api.siliconflow.cn',
        Icon: SiliconFlowIcon,
    },
    {
        id: 'azure',
        type: ChannelType.OpenAIChat,
        baseUrl: '',
        Icon: AzureIcon,
    },
];
