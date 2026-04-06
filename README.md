# Contract Review

一套合同相关的 Web 应用：后端REST 与 SSE 接口，前端用 Next.js。核心能力包括合同上传与解析、基于多 Agent 编排的智能审阅&& RAG 检索审阅规范、以及两份合同的比对。 LLM 编排，对之前诗山的重塑之物。

## 功能概览

- **用户与鉴权**：注册、登录、JWT、刷新 Token；部分接口需携带 Token。
- **会话**：按类型（审阅 / 比对等）创建、列表、历史详情、改标题、删除。
- **合同**：上传 PDF/DOCX/TXT，解析甲乙方等信息；列表、下载、合同类型管理。
- **审阅**：创建审阅任务，SSE 流式返回风险点与修改建议；当前服务端走 Agent 编排（条款拆分、风险、建议、质量门等），RAG 从 MySQL 知识库表加载关键词索引（向量库为可选扩展）。
- **比对**：两份 DOCX 的段落与字符级差异、相似度。
- **其它**：前端还依赖看板、模型配置、聊天等接口时，后端提供了占位返回，可按需替换为真实实现。

更细的架构说明见仓库内 `docs/contract-review-agent-architecture.md`。

## 技术栈（后端）

主要采用了主从Agent的编排方式

## 目录结构（简要）

```
app/
  internal/          # 业务：user、session、contract、review、comparison、agent、rag、knowledge 等
  internal/cmd/      # HTTP 入口与路由注册
  config/            # 配置文件示例与本地配置（勿提交密钥）
frontend/            # Next.js 前端，经 /api/proxy 转发到后端 /api
docs/                # 架构与 SQL 脚本等
```

## 运行前准备

1. **MySQL、Redis，向量数据库等**：创建空库，账号密码写入配置文件。
2. **配置文件**：程序默认读取运行目录下的 `config/conf-dev.yaml`（环境变量 `DEBUG=true` 时）或 `config/conf-pro.yaml`（`DEBUG` 非 true）。可从 `app/config/config.example.yaml` 复制改名，并补齐数据库、Redis、LLM 的 `api_key` 等。
3. **审阅知识库（可选）**：执行 `docs/sql/review_knowledge.sql` 可建表并插入示例数据，供 RAG 关键词检索使用。

首次启动会在连接的数据库上执行 GORM 自动迁移（含审阅知识库表）。

## 启动后端

在项目根目录（保证 `config/` 与可执行文件工作目录一致，或按实际路径调整配置）：

```bash
cd /path/to/Contract_review
export DEBUG=true
go run ./app/internal/cmd
```

监听地址与端口以配置文件中的 `server.host`、`server.port` 为准。

## 启动前端

```bash
cd frontend
npm install
npm run dev
```

前端通过环境变量中的后端地址（如 `NEXT_PUBLIC_DEV_URL`）将 `/api/proxy/*` 转发到后端，需与后端实际监听地址一致。

## 许可证与说明

个人学习与实验用途为主；生产环境使用前请自行加固安全、审计配置与依赖，并替换所有占位接口与默认密钥。
