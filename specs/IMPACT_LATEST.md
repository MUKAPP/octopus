## Target

为渠道增加全局优先级，并在倍率优先模式下将其作为同倍率候选的排序依据。

## Dependents (8)

- `internal/model/channel.go`: 渠道持久化模型和更新请求。
- `internal/op/channel.go`: 渠道创建、校验、更新和缓存刷新。
- `internal/model/group.go`: 运行时候选项承载渠道优先级。
- `internal/op/group.go`: 从渠道缓存向分组候选映射倍率与渠道优先级。
- `internal/relay/balancer/balancer.go`: 倍率优先排序规则。
- `web/src/api/endpoints/channel.ts`: 前端渠道数据契约。
- `web/src/components/modules/channel/Form.tsx`: 渠道优先级输入控件。
- `web/src/components/modules/channel/Create.tsx` 与 `CardContent.tsx`: 创建和编辑数据流。

## Affected Stories

- 当前仓库没有 `specs/release-plan.yaml` 或 epic capsule；本次扩展渠道配置与倍率优先调度规则。

## Test Coverage

- `internal/relay/balancer/balancer_test.go`: 覆盖倍率、渠道优先级和分组项优先级的排序顺序。
- `internal/op/group_test.go`: 覆盖渠道优先级到运行时候选项的映射。
- `internal/op/channel_test.go`: 覆盖渠道优先级更新与负值校验。
- 前端没有组件测试；通过 TypeScript、ESLint 和诊断检查验证表单数据契约。

## Risk: Medium

变更跨越数据库模型、渠道设置、分组候选映射和调度排序，但新增字段默认值为 0，倍率不同的排序以及故障转移模式均保持不变。

## Recommended action

继续实施；同倍率时先比较渠道优先级，渠道优先级相同时再沿用分组项优先级，确保兼容现有分组顺序。
