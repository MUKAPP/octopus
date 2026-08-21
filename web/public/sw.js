// Service Worker for Octopus PWA
// Vite: hashed assets under /assets/ are immutable (Cache First)

/**
 * Cache naming
 * - Prefix MUST match `web/src/lib/sw.ts` (OCTOPUS_CACHE_PREFIX)
 * - Bump CACHE_VERSION when you change caching behavior in this file
 * - FONT cache is version-independent (fonts persist across updates)
 */
const CACHE_PREFIX = 'octopus';
const CACHE_VERSION = 'v2';
const CACHE_NAMES = {
    shell: `${CACHE_PREFIX}-shell-${CACHE_VERSION}`,
    static: `${CACHE_PREFIX}-static-${CACHE_VERSION}`,
    // Font cache is NOT versioned - persists across app updates
    font: `${CACHE_PREFIX}-font`,
};

const SW_MESSAGE_TYPE = {
    SKIP_WAITING: 'SKIP_WAITING',
    CLEAR_CACHE: 'CLEAR_CACHE',
    CACHE_CLEARED: 'CACHE_CLEARED',
};

// 固定 PWA 资源；缺失时单独跳过，不应阻止 Service Worker 安装。
const CORE_ASSETS = [
    '/manifest.json',
    '/favicon.ico',
    '/apple-icon.png',
    '/web-app-manifest-192x192.png',
    '/web-app-manifest-512x512.png',
    '/logo.svg',
    '/logo-dark.svg',
];

const STATIC_DESTINATIONS = new Set(['script', 'style', 'image', 'font', 'manifest']);
const STATIC_RESOURCE_PATTERN = /\.(?:css|js|mjs|png|jpg|jpeg|webp|gif|svg|ico|woff2?|ttf|otf|json|webmanifest)$/i;

// ============ 安装事件 ============
self.addEventListener('install', (event) => {
    event.waitUntil(
        (async () => {
            // Best-effort precache: a missing optional asset must not block installation.
            try {
                await cacheAppShell();
            } catch {
                // Keep installation best-effort for incomplete deployments.
            }
            await self.skipWaiting();
        })()
    );
});

// ============ 激活事件 ============
self.addEventListener('activate', (event) => {
    event.waitUntil(
        (async () => {
            // Clean up old Octopus caches (previous versions), then take control.
            await deleteOctopusCaches({ keep: new Set(Object.values(CACHE_NAMES)) });
            await self.clients.claim();
        })()
    );
});

// ============ Fetch 事件 ============
self.addEventListener('fetch', (event) => {
    const { request } = event;
    const url = new URL(request.url);

    // 只处理同源 GET 请求
    if (url.origin !== self.location.origin || request.method !== 'GET') {
        return;
    }

    // 跳过 API、Service Worker 和 Vite 开发环境资源
    if (isBypassedUrl(url)) {
        return;
    }

    // 字体资源：Cache First（永久缓存，跨版本持久化）
    if (isFontUrl(url)) {
        event.respondWith(cacheFirst(request, CACHE_NAMES.font));
        return;
    }

    // /assets/ 资源：Cache First（带哈希，永不变）
    if (url.pathname.startsWith('/assets/')) {
        event.respondWith(cacheFirst(request, CACHE_NAMES.static));
        return;
    }

    // 页面导航：Network First，离线时返回缓存的首页
    if (request.mode === 'navigate') {
        event.respondWith(networkFirst(request, CACHE_NAMES.shell, { fallbackUrl: '/' }));
        return;
    }

    // 只缓存同源静态资源，避免把任意 GET 请求或动态响应写入缓存。
    if (!isStaticResourceUrl(url, request)) {
        return;
    }

    // 其他静态资源（public 目录）：Stale While Revalidate
    staleWhileRevalidate(event, request, CACHE_NAMES.shell);
});

// ============ Shell precache ============

/**
 * 从当前根 HTML 中提取构建入口、modulepreload、样式和 public 资源。
 * 仅保留同源静态资源；外部资源、API 与 Vite 开发资源不会被缓存。
 */
function extractShellAssets(html) {
    const assets = new Set(CORE_ASSETS);

    for (const match of html.matchAll(/\b(?:href|src)=["']([^"'#]+)["']/gi)) {
        try {
            const url = new URL(match[1], `${self.location.origin}/`);
            if (
                url.origin !== self.location.origin ||
                isBypassedUrl(url) ||
                !isStaticResourceUrl(url)
            ) {
                continue;
            }
            assets.add(`${url.pathname}${url.search}`);
        } catch {
            // Ignore malformed or unsupported HTML attributes.
        }
    }

    return [...assets];
}

/**
 * 缓存首页和当前构建引用的资源。每个资源独立请求，缺失的可选资源只会被跳过。
 */
async function cacheAppShell() {
    const response = await fetch('/', { cache: 'no-store' });
    if (!response.ok) {
        return;
    }

    const shellCache = await caches.open(CACHE_NAMES.shell);
    const staticCache = await caches.open(CACHE_NAMES.static);
    await shellCache.put('/', response.clone());

    let html = '';
    try {
        html = await response.clone().text();
    } catch {
        // The fixed core assets are still useful when HTML parsing is unavailable.
    }

    const assets = extractShellAssets(html);
    await Promise.all(
        assets.map(async (asset) => {
            try {
                const url = new URL(asset, self.location.origin);
                if (
                    url.origin !== self.location.origin ||
                    isBypassedUrl(url) ||
                    !isStaticResourceUrl(url)
                ) {
                    return;
                }

                const assetResponse = await fetch(url, { cache: 'no-store' });
                if (!assetResponse.ok) {
                    return;
                }

                const cache = url.pathname.startsWith('/assets/') ? staticCache : shellCache;
                await cache.put(url, assetResponse.clone());
            } catch {
                // Optional PWA/public assets may not exist in every deployment.
            }
        })
    );
}

// ============ 缓存策略 ============

/**
 * Cache First：优先缓存，适用于带哈希的不变资源
 */
async function cacheFirst(request, cacheName) {
    const cache = await caches.open(cacheName);
    const cached = await cache.match(request);
    if (cached) {
        return cached;
    }

    try {
        const response = await fetch(request);
        if (response.ok) {
            try {
                await cache.put(request, response.clone());
            } catch {
                // A full or restricted cache must not hide the network response.
            }
        }
        return response;
    } catch {
        // 离线且无缓存
        return new Response('Offline', { status: 503 });
    }
}

/**
 * Network First：优先网络，适用于页面导航
 */
async function networkFirst(request, cacheName, { fallbackUrl = null } = {}) {
    const cache = await caches.open(cacheName);
    try {
        const response = await fetch(request);
        if (response.ok) {
            try {
                await cache.put(request, response.clone());
            } catch {
                // Return the network response even if it cannot be cached.
            }
        }
        return response;
    } catch {
        const cached = await cache.match(request);
        if (cached) {
            return cached;
        }
        // 如果有 fallback（通常是首页），返回 fallback
        if (fallbackUrl) {
            const fallback = await cache.match(fallbackUrl);
            if (fallback) return fallback;
        }
        return new Response('Offline', { status: 503 });
    }
}

/**
 * Stale While Revalidate：返回缓存同时后台更新
 */
function staleWhileRevalidate(event, request, cacheName) {
    const cachePromise = caches.open(cacheName);
    const updatePromise = cachePromise.then(async (cache) => {
        const response = await fetch(request);
        if (response.ok) {
            try {
                await cache.put(request, response.clone());
            } catch {
                // Return the network response even if it cannot be cached.
            }
        }
        return response;
    });

    // Keep the background refresh alive after returning a cached response.
    event.waitUntil(updatePromise.then(() => undefined).catch(() => undefined));
    event.respondWith(
        cachePromise
            .then(async (cache) => {
                const cached = await cache.match(request);
                if (cached) {
                    return cached;
                }
                return updatePromise;
            })
            .catch(() => new Response('Offline', { status: 503 }))
    );
}

// ============ 消息事件 ============
self.addEventListener('message', (event) => {
    const { type } = event.data || {};

    switch (type) {
        case SW_MESSAGE_TYPE.SKIP_WAITING:
            self.skipWaiting();
            break;

        case SW_MESSAGE_TYPE.CLEAR_CACHE:
            // Only clear Octopus caches (avoid nuking other same-origin caches).
            // PRESERVE font cache - fonts should persist across updates.
            event.waitUntil(
                (async () => {
                    await deleteOctopusCaches({ keep: new Set([CACHE_NAMES.font]) });
                    const clients = await self.clients.matchAll();
                    clients.forEach((client) => client.postMessage({ type: SW_MESSAGE_TYPE.CACHE_CLEARED }));
                })()
            );
            break;
    }
});

// ========= Helpers =========
function isBypassedUrl(url) {
    const { pathname } = url;
    return (
        pathname === '/sw.js' ||
        pathname === '/api' ||
        pathname.startsWith('/api/') ||
        pathname === '/v1' ||
        pathname.startsWith('/v1/') ||
        pathname.startsWith('/@vite') ||
        pathname.startsWith('/@react-refresh')
    );
}

function isFontUrl(url) {
    return /\.(?:woff2?|ttf)$/i.test(url.pathname);
}

function isStaticResourceUrl(url, request = null) {
    return (
        url.pathname.startsWith('/assets/') ||
        (request && STATIC_DESTINATIONS.has(request.destination)) ||
        STATIC_RESOURCE_PATTERN.test(url.pathname)
    );
}

function isOctopusCacheName(name) {
    return name.startsWith(`${CACHE_PREFIX}-`);
}

async function deleteOctopusCaches({ keep } = {}) {
    const names = await caches.keys();
    const deletions = names
        .filter((name) => isOctopusCacheName(name))
        .filter((name) => !(keep && keep.has(name)))
        .map((name) => caches.delete(name));
    await Promise.all(deletions);
}
