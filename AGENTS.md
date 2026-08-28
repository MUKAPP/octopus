# AGENTS.md

适用于仓库根目录及其所有子目录。

## 仓库关系

- 本仓库是 `MUKAPP/octopus` 的 fork，已在上游基础上修改部分功能；上游仓库地址为 `https://github.com/bestruirui/octopus.git`。

## 项目结构

- `main.go`、`cmd/`：Cobra 命令入口；服务通过 `go run main.go start` 启动。
- `internal/`：后端领域实现。`server/` 提供 HTTP 路由和中间件，`relay/` 处理上游 API 转发，`op/` 编排业务操作，`model/` 定义持久化模型，`db/` 管理数据库，`conf/` 管理运行配置。
- `web/`：React 19、TypeScript、Vite 前端；`src/api/` 为 API 客户端，`src/components/` 为界面，`src/locales/` 为所有用户可见文本的翻译。
- `static/out/`：前端构建产物。由 `static/static.go` 通过 `go:embed` 嵌入；不得手工修改。
- `data/`：默认运行配置和 SQLite 数据库；包含本地状态，除非任务明确要求，不要重置、替换或提交其中的数据。
- `scripts/build.sh`：发布构建脚本；`docker-compose.yaml`：容器部署示例。

## 环境与命令

- `go.mod` 声明 Go `1.26.0`，通过远程模块依赖 `github.com/looplj/axonhub/llm`；GitHub Actions 在构建前执行 `go get github.com/looplj/axonhub/llm@unstable`，构建时以该命令解析的上游最新版本为准。仅更新某个伪版本的上游依赖提交通常不需要单独合入；本地确需验证最新版时使用同一命令，不要手工修改 `go.sum`。
- 前端使用 pnpm，锁文件为 `web/pnpm-lock.yaml`。在 `web/` 中执行：
  - `pnpm install`：安装依赖。
  - `pnpm dev`：启动 Vite 开发服务器；可用 `VITE_PROXY_TARGET` 改写后端代理目标。
  - `pnpm lint`：运行 ESLint。
  - `pnpm build`：先执行 TypeScript 检查，再将产物写入 `../static/out`。
- 后端常用命令：
  - `go test ./...`：运行后端测试。
  - `go run main.go start`：以默认 `data/config.json` 启动服务；可使用 `--config <path>` 指定配置。
- 前端依赖后端 API。联调时先启动后端，再在 `web/` 使用 `VITE_PROXY_TARGET="http://127.0.0.1:8080" pnpm dev`。改动嵌入式前端前，先运行 `pnpm build`。

## 上游同步边界

- 不合并上游改变请求调度方式的提交。当前 fork 保留独立的调度器、balancer、渠道/分组模型和相关数据库结构。
- 上游故障转移、倍率/优先级调度、渠道模型拆表等调度或 schema 改动，以及依赖它们的 relay、超时、统计、迁移和前端提交，均不要与当前 fork 混合合并，除非任务明确要求迁移。

## 开发约定

- 保持 Go 代码按 `gofmt` 格式化，遵循现有包边界；不要让 HTTP handler 承担 `op/`、`relay/` 或数据库领域逻辑。
- 修改导出 Go 标识符前，先检查所有调用点；行为变化应补充或更新相邻包中的 `_test.go` 测试。
- 前端沿用 `@/` 路径别名、现有组件目录和 TypeScript 类型。复用 `src/api/` 的客户端与 endpoint 定义，不在组件中另建 API 调用层。
- 所有面向用户的前端文本必须同步维护 `web/src/locales/en.json`、`zh_hans.json` 与 `zh_hant.json`，通过现有 `use-intl` 机制使用；不要硬编码新文案。
- 已有动效使用 `motion/react`，并保留 `MotionConfig reducedMotion="user"` 与现有 reduced-motion 分支。
- 不提交依赖目录、构建产物、临时数据库、`.env*`、密钥或访问令牌。不要手工编辑 `go.sum` 或 `web/pnpm-lock.yaml`；仅在对应依赖变更后由工具更新。

## 验证与变更范围

- 后端改动至少运行受影响包的 `go test`；跨包、路由、配置或模型改动运行 `go test ./...`。
- 前端改动至少运行 `pnpm lint` 和 `pnpm build`；涉及交互或视觉效果时，在浏览器中验证改动路径。
- 每个 PR 只包含一个功能或修复主题，并在提交前人工审查 AI 辅助改动，遵循 `CONTRIBUTING.md`。
