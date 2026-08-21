import type { ComponentType } from 'react';
import type { SvgIconProps } from '@thesvg/react';
import OpenAIIcon from '@thesvg/react/openai';
import ClaudeIcon from '@thesvg/react/claude';
import GeminiIcon from '@thesvg/react/gemini';
import DeepSeekIcon from '@thesvg/react/deepseek';
import MistralIcon from '@thesvg/react/mistral';
import QwenIcon from '@thesvg/react/qwen';
import MetaIcon from '@thesvg/react/meta';
import CohereIcon from '@thesvg/react/cohere';
import PerplexityIcon from '@thesvg/react/perplexity';
import ZhipuIcon from '@thesvg/react/zhipu';
import YiIcon from '@thesvg/react/yi';
import KimiIcon from '@thesvg/react/kimi';
import MinimaxIcon from '@thesvg/react/minimax';
import DoubaoIcon from '@thesvg/react/doubao';
import HunyuanIcon from '@thesvg/react/hunyuan';
import SparkIcon from '@thesvg/react/spark';
import WenxinIcon from '@thesvg/react/wenxin';
import NvidiaIcon from '@thesvg/react/nvidia-nemotron';
import GrokIcon from '@thesvg/react/grok-xai';
import GoogleIcon from '@thesvg/react/google';
import InternLMIcon from '@thesvg/react/internlm';
import StepfunIcon from '@thesvg/react/stepfun';
import GemmaIcon from '@thesvg/react/gemma-google';
import MicrosoftIcon from '@thesvg/react/microsoft';
import KwaiKATIcon from '@thesvg/react/kwaikat-kat-coder';
import XiaomiMimoIcon from '@thesvg/react/xiaomi-mimo';
import AzureIcon from '@thesvg/react/azure';
import AwsIcon from '@thesvg/react/aws';
import TogetherIcon from '@thesvg/react/together-ai';
import FireworksIcon from '@thesvg/react/fireworks';
import ReplicateIcon from '@thesvg/react/replicate';
import HuggingFaceIcon from '@thesvg/react/hugging-face';
import GroqIcon from '@thesvg/react/groq';
import OllamaIcon from '@thesvg/react/ollama';
import OpenRouterIcon from '@thesvg/react/openrouter';
import CloudflareIcon from '@thesvg/react/cloudflare';
import CerebrasIcon from '@thesvg/react/cerebras';
import SambaNovaIcon from '@thesvg/react/sambanova';
import NovitaIcon from '@thesvg/react/novita';
import VolcengineIcon from '@thesvg/react/volcengine';
import SiliconCloudIcon from '@thesvg/react/siliconcloud-siliconflow';

type IconComponent = ComponentType<SvgIconProps>;

type ModelIconConfig = {
    prefixes: string[];
    Icon: IconComponent;
    className?: string;
    color: string;
};

// Provider patterns intentionally match the model suffix after the first slash,
// preserving the fork's support for channel/model names such as "qwen/gpt-4".
const MODEL_ICON_PATTERNS: ModelIconConfig[] = [
    { prefixes: ['gpt-', 'o1', 'o3', 'o4', 'chatgpt', 'text-embedding', 'dall-e', 'openai'], Icon: OpenAIIcon, className: 'brightness-0 dark:invert', color: '#10A37F' },
    { prefixes: ['claude', 'anthropic'], Icon: ClaudeIcon, color: '#D7765A' },
    { prefixes: ['gemini'], Icon: GeminiIcon, color: '#4285F4' },
    { prefixes: ['gemma'], Icon: GemmaIcon, color: '#4285F4' },
    { prefixes: ['palm', 'google'], Icon: GoogleIcon, color: '#4285F4' },
    { prefixes: ['xiaomi', 'mimo'], Icon: XiaomiMimoIcon, color: '#FF6900' },
    { prefixes: ['deepseek'], Icon: DeepSeekIcon, color: '#4D6BFE' },
    { prefixes: ['grok', 'xai'], Icon: GrokIcon, color: '#000000' },
    { prefixes: ['qwen', 'qwq', 'alibaba'], Icon: QwenIcon, className: 'brightness-0 dark:invert', color: '#6B4EFF' },
    { prefixes: ['glm', 'chatglm', 'zhipu', 'z-ai'], Icon: ZhipuIcon, color: '#3C5BFC' },
    { prefixes: ['minimax', 'abab'], Icon: MinimaxIcon, color: '#1A1A2E' },
    { prefixes: ['moonshot', 'kimi'], Icon: KimiIcon, color: '#000000' },
    { prefixes: ['mistral', 'mixtral', 'codestral', 'pixtral'], Icon: MistralIcon, color: '#F7D046' },
    { prefixes: ['llama', 'meta-llama', 'meta'], Icon: MetaIcon, color: '#0668E1' },
    { prefixes: ['doubao', 'skylark', 'bytedance'], Icon: DoubaoIcon, color: '#00D6C2' },
    { prefixes: ['yi-', '01-ai'], Icon: YiIcon, color: '#1B1464' },
    { prefixes: ['hunyuan'], Icon: HunyuanIcon, color: '#0052D9' },
    { prefixes: ['spark'], Icon: SparkIcon, color: '#0078FF' },
    { prefixes: ['ernie', 'wenxin', 'baidu'], Icon: WenxinIcon, color: '#2932E1' },
    { prefixes: ['internlm'], Icon: InternLMIcon, color: '#2F54EB' },
    { prefixes: ['stepfun', 'step-'], Icon: StepfunIcon, color: '#5B5CFF' },
    { prefixes: ['nvidia', 'nemotron'], Icon: NvidiaIcon, color: '#76B900' },
    { prefixes: ['azure'], Icon: AzureIcon, color: '#0078D4' },
    { prefixes: ['aws', 'amazon', 'bedrock'], Icon: AwsIcon, color: '#FF9900' },
    { prefixes: ['volcengine'], Icon: VolcengineIcon, color: '#3370FF' },
    { prefixes: ['siliconflow', 'siliconcloud'], Icon: SiliconCloudIcon, color: '#7C3AED' },
    { prefixes: ['groq'], Icon: GroqIcon, color: '#F55036' },
    { prefixes: ['together'], Icon: TogetherIcon, color: '#0F6FFF' },
    { prefixes: ['fireworks'], Icon: FireworksIcon, color: '#FF6B00' },
    { prefixes: ['replicate'], Icon: ReplicateIcon, color: '#000000' },
    { prefixes: ['ollama'], Icon: OllamaIcon, color: '#FFFFFF' },
    { prefixes: ['openrouter'], Icon: OpenRouterIcon, color: '#6366F1' },
    { prefixes: ['cloudflare'], Icon: CloudflareIcon, color: '#F38020' },
    { prefixes: ['cerebras'], Icon: CerebrasIcon, color: '#FF5722' },
    { prefixes: ['sambanova'], Icon: SambaNovaIcon, color: '#FF6B00' },
    { prefixes: ['novita'], Icon: NovitaIcon, color: '#7C3AED' },
    { prefixes: ['huggingface', 'hugging-face', 'hf'], Icon: HuggingFaceIcon, color: '#FFD21E' },
    { prefixes: ['cohere', 'command'], Icon: CohereIcon, color: '#39594D' },
    { prefixes: ['perplexity'], Icon: PerplexityIcon, color: '#20B8CD' },
    { prefixes: ['phi-'], Icon: MicrosoftIcon, color: '#00BCF2' },
    { prefixes: ['kat'], Icon: KwaiKATIcon, color: '#1969FC' },
];

const DEFAULT_CONFIG = { Icon: OpenAIIcon, className: 'brightness-0 dark:invert', color: '#10A37F' };

export function getModelIcon(modelName: string): { Icon: IconComponent; className?: string; color: string } {
    const nameToMatch = modelName.includes('/') ? modelName.slice(modelName.indexOf('/') + 1) : modelName;
    const lowerName = nameToMatch.toLowerCase();
    for (const { prefixes, Icon, className, color } of MODEL_ICON_PATTERNS) {
        if (prefixes.some((prefix) => lowerName.startsWith(prefix))) {
            return { Icon, className, color };
        }
    }
    return DEFAULT_CONFIG;
}
