# 贡献指南

感谢你对 Smile 思迈 项目的关注！欢迎提交 Issue 和 Pull Request。

## 开发环境

- **Go** >= 1.22
- **Git**
- 无需 CGO（使用纯 Go SQLite 驱动）

## 开发流程

1. **Fork** 本仓库到你的 GitHub 账号
2. **克隆**到你本地：
   ```bash
   git clone https://github.com/<your-username>/smile-sql.git
   cd smile-sql
   ```
3. **创建特性分支**：
   ```bash
   git checkout -b feature/your-feature-name
   ```
4. **安装依赖**：
   ```bash
   go mod download
   ```
5. **运行**：
   ```bash
   cp config.yaml.example config.yaml
   go run ./cmd/server/
   ```
6. **编码** → 测试 → 提交
7. **推送**到你的 Fork：
   ```bash
   git push origin feature/your-feature-name
   ```
8. 在 GitHub 上创建 **Pull Request**

## 代码规范

- 遵循 [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- 使用 `gofmt` 格式化代码
- 新增功能需更新对应的测试
- 提交信息使用英文，格式：`<type>: <description>`（如 `feat: add pgsql support`）

## 提交类型

| 类型 | 说明 |
|------|------|
| `feat` | 新功能 |
| `fix` | Bug 修复 |
| `docs` | 文档更新 |
| `refactor` | 代码重构 |
| `test` | 测试相关 |
| `chore` | 构建/工具相关 |

## 项目架构

```
Web 层 (HTMX) → 业务层 (internal/) → 执行引擎 / Git 集成 → 数据层 (SQLite)
```

- **Web 层**：`internal/web/`，HTTP 路由和模板渲染
- **业务层**：`internal/repo/`，数据访问；`internal/crypto/`，加密；`internal/config/`，配置
- **执行引擎**：`internal/executor/`，SQL 执行和连接测试
- **Git 集成**：`internal/git/`，仓库 clone/commit/push
- **数据层**：`internal/db/`，数据库初始化和迁移

## 目录约定

- `cmd/` — 程序入口
- `internal/` — 业务逻辑（不对外暴露）
- `migrations/` — 数据库迁移脚本
- `web/templates/` — HTML 模板
- `web/static/` — 静态资源
- `docs/` — 文档

## Issue 指南

提交 Issue 时请包含：

1. **问题描述**：清晰描述遇到的问题
2. **复现步骤**：详细的重现步骤
3. **期望行为**：你期望发生什么
4. **实际行为**：实际发生了什么
5. **环境信息**：操作系统、Go 版本、浏览器等
