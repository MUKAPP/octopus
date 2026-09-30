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

`src/globals.css` 在 `@layer base` 对 `*` 设置 `scrollbar-width: none`，原生滚动条默认全部隐藏；只有真实可滚动的容器显式加上 `.scrollbar` 才显示细条指示。`.scrollbar` 的拇指色由主题前景色经 `color-mix` 推导，亮/暗主题自动适配。`scrollbar-gutter: stable both-edges` 在左右对称预留槽位，滚动条出现与否都保持同一内容宽度与居中轴，不因右侧滚动条使布局向左偏移。

新增滚动区域时：

- 只给真正会产生滚动的容器加 `.scrollbar`（`overflow-y-auto` / `overflow-x-auto` / 会溢出的 `overflow-auto`）。
- 不要给纯装饰性裁切容器加（`app-shell.tsx`、`channel/CardContent.tsx` 的卡片、`tabs.tsx` 的 `overflow: hidden`、`badge` / `progress` 等），`scrollbar-gutter` 在 `overflow: hidden` 下同样会占位。
- 内容与滚动条要保持至少 8px 间距：容器没有水平内边距时补对称的 `px-2`（`p-4` / `p-5` 已足够，不必再补），避免只加 `pr-2` 导致左偏；横向滚动的容器补 `pb-2`。
- 热力图等横向滚动区域同时保留渐隐 mask 和底部滚动条，并用底部内边距隔开内容。

MorphingDialog（`src/components/ui/morphing-dialog.tsx`）的视口容器用 `overflow-clip` 限制可见范围，避免焦点或 `scrollIntoView` 滚动容器本身。内容仍可能裁切溢出的子元素，因此浮层不要依赖绝对定位越出宿主卡片。设置中的密钥与账号表单通过 `MorphingDialogOverlayLayer` 挂到 `document.body`，由浮层自己承担视口高度约束、滚动和嵌套键盘处理；外点不丢弃草稿，提交期间禁用取消操作和 Escape 关闭。

移动端通过 `viewport-fit=cover` 让背景铺到边缘；`src/globals.css` 的四向 `--safe-area-*` token 统一约束内容。底部导航用动态 bottom inset 移位，滚动内容用 `pb-nav-clearance` / `h-nav-clearance` 留出导航净空；AppShell 本身不重复添加底部 inset。两类 MorphingDialog portal 使用 `--overlay-*` 四向边界，可用高度只由外层扣除一次，内部面板继续 `max-h-full` 并自行滚动。

Popover、Select 与价格编辑通过 `src/hooks/use-safe-area-insets.ts` 共享 resize / visualViewport.resize 的数值快照。碰撞边界累加调用者原有间距；Select 原有 4px 视觉间距通过 Radix sideOffset 参与碰撞计算，不使用计算后的位移越过安全边界。价格编辑内容过高时在浮层内滚动，保存按钮仍可触达。

Toast 保留调用者显式 `offset` / `mobileOffset` 的优先级。Sonner 的移动端默认样式只按左侧 offset 计算卡片宽度，因此 `globals.css` 在其 600px 移动断点内将容器宽度改为分别扣除左右 mobile offset，卡片填满该容器；不对称 cutout、left/right/center 位置均使用同一可用宽度，桌面宽度与定位不变。
