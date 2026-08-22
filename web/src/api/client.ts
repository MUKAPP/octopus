type RequestMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';

export type RequestParams = Record<string, string | number | boolean>;

export type RequestOptions = {
    method?: RequestMethod;
    body?: unknown;
    params?: RequestParams;
    headers?: HeadersInit;
    dispatchUnauthorized?: boolean;
    signal?: AbortSignal;
};

export class ApiError extends Error {
    readonly status: number;
    readonly code: number;

    constructor(status: number, message: string) {
        super(message);
        this.name = 'ApiError';
        this.status = status;
        this.code = status;
    }
}

export const apiUnauthorizedEvent = 'api:unauthorized';

let apiKey: string | null = null;

export function setAPIKey(value: string | null) {
    apiKey = value;
}

function isJsonBody(value: unknown): boolean {
    if (value === null || typeof value !== 'object') return typeof value !== 'string';
    if (typeof FormData !== 'undefined' && value instanceof FormData) return false;
    if (typeof Blob !== 'undefined' && value instanceof Blob) return false;
    if (typeof URLSearchParams !== 'undefined' && value instanceof URLSearchParams) return false;
    if (typeof ArrayBuffer !== 'undefined' && value instanceof ArrayBuffer) return false;
    return true;
}

function appendParams(path: string, params?: RequestParams): string {
    if (!params) return path;
    const query = new URLSearchParams(
        Object.entries(params).map(([key, value]) => [key, String(value)])
    ).toString();
    if (!query) return path;
    return `${path}${path.includes('?') ? '&' : '?'}${query}`;
}

async function readResponse(response: Response): Promise<unknown> {
    if (response.status === 204) return null;
    const contentType = response.headers.get('content-type') ?? '';
    if (contentType.includes('application/json')) {
        return response.json().catch(() => null);
    }
    return response.text();
}

export async function apiRequest<T>(path: string, options: RequestOptions = {}): Promise<T> {
    const headers = new Headers(options.headers);
    let body: BodyInit | undefined;

    if (options.body !== undefined) {
        if (isJsonBody(options.body)) {
            headers.set('Content-Type', 'application/json');
            body = JSON.stringify(options.body);
        } else {
            body = options.body as BodyInit;
        }
    }

    if (apiKey) headers.set('Authorization', `Bearer ${apiKey}`);

    const response = await fetch(appendParams(path, options.params), {
        method: options.method ?? 'GET',
        headers,
        body,
        credentials: 'include',
        signal: options.signal,
    });
    const data = await readResponse(response) as { message?: unknown; data?: unknown } | unknown;

    if (!response.ok) {
        const record = data && typeof data === 'object' ? data as Record<string, unknown> : null;
        const message = typeof record?.message === 'string'
            ? record.message
            : typeof data === 'string' && data
                ? data
                : response.statusText || `Request failed: ${response.status}`;
        if (response.status === 401 && options.dispatchUnauthorized !== false && typeof window !== 'undefined') {
            window.dispatchEvent(new Event(apiUnauthorizedEvent));
        }
        throw new ApiError(response.status, message);
    }

    if (data && typeof data === 'object' && 'data' in data) {
        return (data as { data?: T }).data as T;
    }
    return data as T;
}
