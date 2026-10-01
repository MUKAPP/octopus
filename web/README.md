# Octopus Web

前端使用 React、TypeScript 和 Vite。

开发环境启动：

```bash
pnpm install
pnpm dev
```

Vite 默认监听 `http://localhost:5173`，并将 `/api` 请求代理到 `http://127.0.0.1:8080`。如需连接其他后端地址，可在启动时设置 `VITE_PROXY_TARGET`：

```bash
VITE_PROXY_TARGET="http://127.0.0.1:8080" pnpm dev
```

生产构建：

```bash
pnpm build
```

构建产物直接输出到 `static/out`，供 Go 二进制文件嵌入。

## 滚动与浮层布局

`src/globals.css` 在 `@layer base` 对 `*` 设置 `scrollbar-width: none`，原生滚动条默认全部隐藏；只有真实可滚动的容器显式加上 `.scrollbar`，作为由增强轨道接管的标记。`src/lib/overlay-scrollbars.ts` 与 `src/components/common/OverlayScrollbars.tsx` 在 `main.tsx` 的 Provider 下安装一次，识别所有带 `.scrollbar` 的原生滚动容器，用绝对定位的轨道覆盖原生滚动条位置，同时覆盖纵向、横向以及 `textarea`。`src/globals.css` 仍提供轨道的拇指色（由主题前景色经 `color-mix` 推导，亮/暗主题自动适配），增强本身为本地实现，未引入新依赖。

轨道不进入文档流、不占用布局尺寸，容器保持原有内容宽度与居中，也不需要靠预留滚动条槽位或补内边距维持对齐；容器原有的 `overflow`、`ref` / `scrollTop` / `scrollLeft` 用法不变，键盘滚动与原生 wheel、触摸滚动仍由原滚动容器处理，轨道支持真实滑块拖动与点击定位。

新增滚动区域时：

- 只给真正会产生滚动的容器加 `.scrollbar`（`overflow-y-auto` / `overflow-x-auto` / 会溢出的 `overflow-auto`）。
- 不要给纯装饰性裁切容器加（`app-shell.tsx`、`channel/CardContent.tsx` 的卡片、`tabs.tsx` 的 `overflow: hidden`、`badge` / `progress` 等），它们没有真实滚动条可接管。
- 不要再为滚动条补 `px-2` / `pr-2` / `pb-2` 一类 gutter 补偿，保留容器原有业务内边距即可；轨道优先使用滚动容器自身已有的内边距，不足时再使用最近祖先既有的外侧留白，不跨越原有滚动或裁切边界。响应式标记（如 `max-md:scrollbar`）只在相应断点绘制轨道。
- 轨道 `aria-hidden`、不设置 `tabIndex`，不新增可聚焦控件，也不改变原有焦点与 Tab 顺序。轨道按下、抬起与点击取消默认行为以保留焦点，但继续传播到文档的外点监听；监听通过 `getOverlayScrollbarViewport(eventTarget)` 判断真实归属，只把自己内部的轨道当作内点，外部轨道仍关闭浮层。Popover、Select 与菜单的轨道留在对应内容树内，保留 Radix 原有内部捕获与点击关闭机制；Ctrl＋滚轮不接管，保留浏览器缩放。
- 轨道 host 作为父层首个绝对定位子节点，显式清零 margin，避免改变 `space-y-*` 的末子节点间距；host 恢复时保持相同插入位置。
- 热力图等横向滚动区域同时保留渐隐 mask 和底部滚动条。

MorphingDialog（`src/components/ui/morphing-dialog.tsx`）的视口容器用 `overflow-clip` 限制可见范围，避免焦点或 `scrollIntoView` 滚动容器本身。内容仍可能裁切溢出的子元素，因此浮层不要依赖绝对定位越出宿主卡片。设置中的密钥与账号表单通过 `MorphingDialogOverlayLayer` 挂到 `document.body`，由浮层自己承担视口高度约束、滚动和嵌套键盘处理；外点不丢弃草稿，提交期间禁用取消操作和 Escape 关闭。

移动端通过 `viewport-fit=cover` 让背景铺到边缘；`src/globals.css` 的四向 `--safe-area-*` token 统一约束内容。底部导航用动态 bottom inset 移位，滚动内容用 `pb-nav-clearance` / `h-nav-clearance` 留出导航净空；AppShell 本身不重复添加底部 inset。两类 MorphingDialog portal 使用 `--overlay-*` 四向边界，可用高度只由外层扣除一次，内部面板继续 `max-h-full` 并自行滚动。

Popover、Select 与价格编辑通过 `src/hooks/use-safe-area-insets.ts` 共享 resize / visualViewport.resize 的数值快照。碰撞边界累加调用者原有间距；Select 原有 4px 视觉间距通过 Radix sideOffset 参与碰撞计算，不使用计算后的位移越过安全边界。价格编辑内容过高时在浮层内滚动，保存按钮仍可触达。

Toast 保留调用者显式 `offset` / `mobileOffset` 的优先级。Sonner 的移动端默认样式只按左侧 offset 计算卡片宽度，因此 `globals.css` 在其 600px 移动断点内将容器宽度改为分别扣除左右 mobile offset，卡片填满该容器；不对称 cutout、left/right/center 位置均使用同一可用宽度，桌面宽度与定位不变。

## 日志活动耗时

`src/api/endpoints/log.ts` 仅在解码 running / committed 网络快照时记录 `performance.now()` 观察锚点，并将服务端整体 duration 从纳秒转换为毫秒。同一请求的活动快照合并先投影到共同观察时刻，避免旧分页或 SSE 导致倒退；终态直接采用服务端值，不把 UI 推算值写回。

日志卡片与活动详情共用 `src/components/modules/log/use-live-duration.ts` 的单个 1 秒时钟。仅有锚点的可见活动指标订阅；页面隐藏时暂停，恢复后按单调时间差补齐，最后一个订阅卸载后清理时钟和监听。开始时刻仅用于时间展示，不参与客户端耗时计算；重试和虚拟列表重挂载不重置总耗时。

日志概览与详情快照还包含可选的 `upstream_model_name` / `response_model_name`。前者取参数覆盖、追加和协议修补之后的真实出站请求，后者只取原始上游响应声明（包括各协议原始流帧），不从转换器结果或候选名补造。字段属于当前尝试，新尝试开始清空，终态以最后尝试结果接管；空值表示未知。两字段仅存于内存日志/JSON，`gorm:"-"` 不新增数据库列，也不恢复旧记录。

只有真实出站模型与原始返回模型均已知且逐字不同时，卡片和详情才显示文字徽标及返回名；候选模型仍用于尝试历史与计价，不将模型差异当作请求失败。完整 pair 同时提供 title / aria-label。长模型名在徽标中换行；短视口详情标题最多占面板内容高度的 40%，可独立滚动查看完整名称，避免挤掉总耗时与其他指标。

## 分组成员首尾移动

分组创建和编辑的已选模型列表提供“移到顶部 / 移到底部”。按钮只修改当前草稿，保存或创建时沿用列表顺序生成 priority；取消不会写入。首项、末项、单成员及正在删除或拖拽的行禁用对应操作。列表卡片不提供这两个按钮，继续使用原有拖拽落点持久化排序。

## 日志渠道密钥标识

每次真实渠道尝试记录 `channel_key_id` 和可选的 `channel_key_remark`。备注取该次使用的 Key 的 `Remark`，在开始尝试时去除首尾空白并保存快照；不记录密钥正文，不拼入渠道名称或改变统计维度。

主卡片显示最后一次非 skipped / circuit_break 的真实尝试标识；重试提示、尝试历史和选中尝试详情分别显示各自的 ID 与备注。旧记录没有备注时仅显示 ID，没有正 ID 时不显示标识；跳过项不会补查备注。长备注换行，重试提示中的尝试列表限制高度并可独立滚动。

重试提示的 `data-scrollable-tooltip` 标记仅用于保留该内容的悬停与内部滚动；离开内容、页面外部滚动或 Escape 仍关闭。未标记的提示保持原有行为。

生产日志当前保存在服务进程的内存 store。修改备注后，已有记录在同一进程内刷新仍显示旧快照，新请求捕获新备注；此字段不新增数据库列，也不承诺服务重启后的历史保留。

## 反向代理子目录与 PWA

生产页面可以部署在 `/`、`/octopus/` 或其他应用目录。`src/lib/base-url.ts` 基于 `document.baseURI` 统一解析 API、导入导出、三条日志 SSE 和 Service Worker URL；构建资源继续使用 Vite 的相对 base。

反向代理必须同时满足：

- 将无末尾斜杠的入口（如 `/octopus`）308 重定向至 `/octopus/`，保留查询参数。
- 将 `/octopus/*` 剥离目录前缀后转发为后端的 `/*`，保留 method、body、query、cookie 和 Authorization。后端仍在根路径挂载路由和嵌入资源。
- SSE 响应不得缓冲；API 不得缓存。导出保留 Content-Disposition，认证保留 Set-Cookie。

Service Worker 注册 scope 为当前应用目录。shell / static 缓存名为 `octopus-${encodeURIComponent(basePath)}-shell-v4` / `static-v4`，各目录独立；`octopus-font` 跨目录共享并保留。仅根目录的 worker 额外清理旧 `octopus-shell-v数字` / `octopus-static-v数字` 缓存，子目录不认领这些旧根缓存。

“强制刷新”由主线程删除当前目录的非字体缓存，并只注销 scope 精确匹配当前目录的注册后刷新；不向可能仍控制页面的旧根 worker 发送清理消息，不删除其他目录或无关产品缓存。离线时只提供已经缓存的页面壳、静态资源和字体，未缓存的 API 仍不可用，既有认证行为不变。

同 origin 下的多目录部署不隔离认证：HttpOnly cookie 仍为 Path=/，登录偏好与 localStorage 仍共享。目录缓存隔离不代表多账户或多后端实例的认证隔离。
