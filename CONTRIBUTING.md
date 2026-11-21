# 参与贡献

感谢你愿意改进 relay-forge。本文说明我们接受什么、怎么提，以及合并前必须满足的条件。

## 先读这些

- [README.md](./README.md)：项目定位与快速开始
- [RELAY-FORGE.md](./RELAY-FORGE.md)：每个新增能力的接口、成本口径与已知限制
- [AGENTS.md](./AGENTS.md)：代码规范（人和 AI 助手都适用）
- [web/AGENTS.md](./web/AGENTS.md)：前端规范
- [SECURITY.md](./.github/SECURITY.md)：安全漏洞的报告方式

## 报告问题

- 先搜索现有 Issue，确认不是重复。
- 使用 Bug 模板，写清：**版本、部署方式、实际现象、影响范围、发生频率、复现步骤、预期结果**。
- 日志和请求必须脱敏：不要贴 API Key、上游凭据、用户数据。
- 第三方托管站、中转站的问题请联系其运营方，这里不受理。
- 不接受整段 AI 生成、未经人工提炼的描述；请自己先读一遍再提交。

## 提功能建议

- 先开 Issue 讨论，方向达成一致再写代码。未经讨论的大改动 PR 可能被关闭。
- 说清使用场景，以及现有做法为什么不够。
- 与「站长收益、客户管理、渠道运维」无关的通用网关能力，优先提给上游 [new-api](https://github.com/QuantumNous/new-api)，避免两边重复实现。

## 提 PR

- 一个 PR 只做一件事；顺带的大范围重构请单独提。
- 关联对应 Issue；设计取舍放在 Issue 里讨论。
- 提交信息用 `类型(范围): 说明`，例如 `fix(clients): 删除 Key 后保留已用量`，中英文均可。
- 合并前必须通过：
  - 后端 `make test`（会分别测试根模块、`relaykit/`、`tokenkit/` 三个 Go 模块）
  - 前端 `cd web && bun run typecheck && bun run test`
  - 改动文件执行 `bun run lint` 与 `bun run format:check`
- 项目里有几条容易踩的硬规则：
  - 新增 admin 路由要登记到 `middleware/access_token_routes.go`，新增渠道字段要归类到 `controller/channel_authz.go`，否则 `router`、`controller` 的守卫测试会失败。
  - 新增日志列要考虑 ClickHouse 日志库：它的建表语句是手写的，不会自动加列。
  - 数据库改动必须同时能在 SQLite、MySQL、PostgreSQL 上工作；涉及迁移的要在三种库上都验证。
- 改了行为就补测试。只提高覆盖率、只证明代码能跑起来的测试不要提。

## 许可与署名

- 本项目遵循 **AGPL-3.0**。
- 不要删除界面和 `NOTICE` 中上游 new-api 的署名与链接——这是上游 AGPLv3 第 7 条附加条款的要求。
- 提交即表示你同意以 AGPL-3.0 授权你的贡献。

## 语言

中文或英文都可以。Issue 与 PR 保持一种语言即可，不要中英混排。

---

## Contributing (English)

Contributions are welcome. Please read [AGENTS.md](./AGENTS.md) and [web/AGENTS.md](./web/AGENTS.md) before writing code.

- Search existing issues first; use the bug or feature template.
- One concern per pull request, linked to an issue.
- Required checks: `make test`, plus `cd web && bun run typecheck && bun run test`; lint and format the files you touched.
- Database changes must work on SQLite, MySQL and PostgreSQL.
- Keep the upstream new-api attribution in the UI and `NOTICE` — the AGPLv3 Section 7 additional terms require it.
- By submitting, you agree to license your contribution under AGPL-3.0.
