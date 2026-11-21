# relay-forge

面向 LLM API 中转站站长的开源网关。在上游 new-api 的基础上，补上站长真正需要的东西：**收益看板、渠道健康巡检、分销持续抽成**，外加**下游客户层**、**单用户封禁**和**一键开站**。

> 独立维护的项目，基于 [new-api](https://github.com/QuantumNous/new-api)（AGPL-3.0）。上游的鉴权、渠道调度、计费与原子扣费逻辑全部保留，本项目只在其上增加能力。

## 它解决什么问题

| 你遇到的 | 这里的做法 |
| --- | --- |
| 转售 API，但算不清每个渠道到底赚多少 | 收益看板：按渠道、按模型看收入、上游成本、毛利和毛利率 |
| 渠道半夜挂了，第二天才发现 | 每 30 分钟健康巡检，按你设定的规则停用、标降智、恢复并通知 |
| 客户是你带来的，却只能拿一次性返利 | 分销持续抽成：下级用户每消费一次，按比例给邀请人记一笔 |
| 客户多了记不清谁用了多少 | 客户层：给每个客户设预算，在其名下签发 Key，毛利 = 预算 − 实际用量 |
| 一个终端用户违规，整把 Key 被上游封掉 | `X-End-User` 头把封禁粒度降到「Key × 模型 × 终端用户」 |
| 装一套要配半天 | 一条命令起站，向导里填完站点信息、首个上游、管理员即可使用 |

## 快速开始

需要 Docker（含 compose 插件）。

```bash
./install.sh            # 默认 3000 端口
PORT=8080 ./install.sh  # 换端口
```

脚本会生成 `.env`（Postgres 与 Redis 的随机密码，权限 600）、拉起服务、轮询 `/api/status` 直到就绪。可以重复运行，已有 `.env` 会被复用——**已经初始化过的数据库不要改密码**。

起完打开 `http://localhost:3000`，向导会依次带你完成：

1. 数据库检查
2. 站点信息（名称、Logo）
3. 首个上游渠道（OpenAI 兼容 / Anthropic）
4. 管理员账号
5. 使用模式
6. 确认与环境自检

镜像默认拉 `ghcr.io/mazihua-lgtm/relay-forge:latest`，可用 `RELAY_FORGE_IMAGE` 覆盖。镜像还没发布过、或本机断网时，脚本会退回使用本地同名镜像，此时先构建：

```bash
docker build -t ghcr.io/mazihua-lgtm/relay-forge:latest .
```

## 界面语言

简体中文、繁體中文、English、Français、日本語、Русский、Tiếng Việt。

管理后台在原有的渠道、令牌、日志之外，新增「收益 / Earnings」和「客户 / Clients」两页。

## 文档

- [CONTRIBUTING.md](./CONTRIBUTING.md)：怎么报问题、提需求、提交 PR，以及合并前必须通过的检查
- [RELAY-FORGE.md](./RELAY-FORGE.md)：每个新增能力的接口、后台入口、成本口径与已知限制
- [NOTICE](./NOTICE)：上游版权声明与 AGPLv3 第 7 条附加条款
- [README.upstream.md](./README.upstream.md)：上游 new-api 的原始说明（归档，用于对照）

## 上游与许可

- 本项目基于 [new-api](https://github.com/QuantumNous/new-api)，遵循 **AGPL-3.0**。
- 按上游要求，界面中保留署名 `Frontend design and development by New API contributors.` 及指向原项目的链接；修改内容按 AGPLv3 §7(c) 标注。
- 以网络服务形式向用户提供修改后的版本时，须向这些用户提供对应源码（AGPL §13）。

---

## English

An open-source gateway for LLM API relay operators, independently maintained on top of [new-api](https://github.com/QuantumNous/new-api) (AGPL-3.0). It keeps the upstream authentication, channel routing and atomic billing, and adds an earnings dashboard, channel health probes, ongoing referral commission, a downstream client layer with budgets, per-end-user policy blocking, and a one-command installer.

Quick start: `./install.sh`, then finish the setup wizard at `http://localhost:3000`. Details in [RELAY-FORGE.md](./RELAY-FORGE.md).
