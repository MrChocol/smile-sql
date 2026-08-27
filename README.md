<p align="center">
  <img src="web/static/logo.jpg" width="120" alt="Smile Logo">
</p>

<h1 align="center">Smile 思迈 · SQL 管理平台</h1>

<p align="center">
  面向多项目交付团队的轻量级 SQL 脚本全生命周期管理平台
</p>

<p align="center">
  <a href="#快速开始">快速开始</a> ·
  <a href="#功能特性">功能特性</a> ·
  <a href="#部署指南">部署</a> ·
  <a href="#项目结构">项目结构</a> ·
  <a href="#技术栈">技术栈</a> ·
  <a href="LICENSE">License</a>
</p>

---

## 简介

**Smile 思迈** 是一款专为多项目并行交付团队设计的 SQL 统一管理平台。它解决的核心痛点是：多项目频繁迭代中 SQL 脚本执行记录混乱、多环境数据源一致性难以保障、Git 归档与版本号管理依赖人工容易遗漏。

平台围绕 **项目 → 版本 → 环境 → 数据源 → 脚本 → 发布 → 推送执行 → 归档** 的完整链路设计，每个环节都有可视化流程引导和状态颜色标识，操作路径清晰直观。

### 核心流程

```
项目配置 → 环境配置 → 数据源配置 → 脚本录入 → 发布编排 → 推送执行 → 归档提交
                                                                ↓
                                                          版本自增 + Git 归档
```

## 功能特性

### 脚本管理
- 平台页面手动录入 SQL 脚本，自动携带创建时间、功能描述、提交人等头部信息
- 脚本创建/编辑即自动同步到 Git 仓库的 `{项目编码}/pending/` 目录
- 支持单个导出和批量合并导出为 `.sql` 文件
- 草稿状态支持编辑，已执行/已归档后锁定

### 多环境推送执行
- 支持 dev / test / prod 多环境数据源配置
- 一键推送执行，支持勾选部分脚本定向推送（失败后可只重跑失败项）
- 执行成功自动标记脚本状态，网络不通时支持手动标记
- 环境执行路线图：以 Git 分支线风格展示各环境最新执行状态

### 执行记录追踪
- `execution_record` 作为唯一数据源，关联脚本、环境、数据源、状态
- 失败原因悬浮可查，以最近一条记录状态为展示基准
- 工作台首页展示最近执行记录与数据统计概览

### Git 归档与版本管理
- 归档提交时脚本写入 `{项目编码}/archived/` 目录，自动生成 commit message
- 版本号支持手动创建与归档自增，来源以颜色+中文区分
- Git 仓库配置（地址、令牌、用户名邮箱）统一管理，保存即热更新
- 空仓库自动初始化，无需人工干预

### 安全与运维
- 数据源密码 AES-256 加密存储，表单内支持明文查看（无需管理密码）
- 数据源测试连通性，AJAX 弹窗提示结果
- 数据清理统一入口，8 个模块独立清理，二次确认+事务回滚保护
- SQLite 内嵌数据库，零外部依赖，单二进制部署

### 用户体验
- 工作台 Dashboard 首页：流程概览 + 快捷操作 + 数据统计 + 最近记录
- 所有核心页面顶部 6 步流程进度条，已完成步骤绿色对勾，当前步骤高亮
- 全表单 AJAX 提交 + 弹窗提示，不刷新页面
- 状态徽章统一颜色体系：绿色=成功、蓝色=已发布、黄色=待执行、红色=失败

## 快速开始

### 方式一：下载预编译二进制（推荐）

前往 [Releases](../../releases) 下载对应平台的压缩包，解压后直接运行：

**Windows:**
```cmd
# 双击 start.bat，或命令行运行
sql-mgr-server.exe
```

**macOS:**
```bash
chmod +x sql-mgr-server
./sql-mgr-server
```

**Linux:**
```bash
chmod +x sql-mgr-server
./sql-mgr-server
```

启动后访问 **http://localhost:8080**，默认账号 `admin` / `admin123`

### 方式二：从源码编译

#### 环境要求

- **Go** >= 1.22
- **Git**（用于归档功能）
- **CGO** 不需要（使用纯 Go SQLite 驱动）

#### 步骤

```bash
# 1. 克隆仓库
git clone <repo-url>
cd smile-sql

# 2. 复制配置文件
cp config.yaml.example config.yaml
# 编辑 config.yaml，修改 encrypt_key 为 32 字节随机字符串：
#   openssl rand -base64 32

# 3. 下载依赖
go mod download

# 4. 编译
go build -o sql-mgr-server ./cmd/server/

# 5. 运行
./sql-mgr-server
```

访问 **http://localhost:8080**，默认账号 `admin` / `admin123`

## 部署指南

### Windows 部署

```cmd
# 1. 解压发行包到任意目录
# 2. 双击 start.bat 启动
# 3. 浏览器访问 http://localhost:8080
```

### macOS 部署

```bash
# 1. 解压
unzip smile-sql-macos.zip
cd smile-sql-mgr

# 2. 启动
chmod +x start.sh
./start.sh
```

### Linux 部署

**快速启动：**
```bash
chmod +x start.sh
./start.sh
```

**systemd 后台运行（生产环境推荐）：**
```bash
# 1. 解压到 /opt
sudo unzip smile-sql-linux.zip -d /opt/
cd /opt/smile-sql-mgr

# 2. 注册 systemd 服务
sudo cp smile-sql.service /etc/systemd/system/
sudo systemctl daemon-reload

# 3. 启动并设置开机自启
sudo systemctl start smile-sql
sudo systemctl enable smile-sql

# 4. 查看状态
sudo systemctl status smile-sql

# 5. 查看日志
sudo journalctl -u smile-sql -f
```

### 离线内网部署

```bash
# 在有网络的机器上：
go mod vendor
GOOS=linux GOARCH=amd64 go build -o sql-mgr-server ./cmd/server/

# 将以下文件复制到内网服务器：
# - sql-mgr-server (二进制)
# - config.yaml
# - migrations/ (目录)
# - web/ (目录)
# - vendor/ (目录，如需重新编译)

# 内网服务器上直接运行：
chmod +x sql-mgr-server
./sql-mgr-server
```

### 配置说明

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `server_port` | HTTP 服务端口 | `8080` |
| `sqlite_path` | SQLite 数据库路径 | `sql-mgr.db` |
| `git_repo_url` | Git 仓库地址（空=禁用） | `""` |
| `git_token_enc` | Git 访问令牌 | `""` |
| `encrypt_key` | AES-256 加密密钥（32 字节） | `change-me-...` |
| `admin_pw_hash` | 管理密码 bcrypt 哈希 | `""` |

> **安全提示：** 生产环境务必修改 `encrypt_key`，建议使用 `openssl rand -base64 32` 生成。
> Git 仓库和令牌也可在平台「系统配置 → Git 配置」页面中设置，将加密存储到数据库。

## 项目结构

```
smile-sql/
├── cmd/
│   └── server/
│       └── main.go              # 程序入口
├── internal/
│   ├── archive/                  # 归档引擎
│   ├── config/                  # 配置加载
│   ├── crypto/                  # AES-256 加密
│   ├── db/                      # 数据库初始化 + 迁移
│   ├── executor/                # SQL 执行引擎
│   ├── git/                     # Git 集成
│   ├── model/                   # 数据模型
│   ├── repo/                    # 数据访问层
│   └── web/                     # Web 层（路由 + 处理器）
├── migrations/
│   └── 0001_init.sql            # 数据库初始化脚本（10 张表）
├── web/
│   ├── static/                  # 静态资源（logo）
│   └── templates/               # HTMX HTML 模板（25 个）
├── docs/                        # 文档
│   ├── deployment-guide.md
│   └── test-checklist.html
├── config.yaml.example          # 配置文件模板
├── go.mod
├── go.sum
├── .gitignore
├── LICENSE
└── README.md
```

## 技术栈

| 层级 | 技术选型 | 说明 |
|------|----------|------|
| 后端 | Go 1.22+ | 单二进制，零 CGO 依赖 |
| 前端 | HTMX + HTML 模板 | 无 SPA，无 CDN，内嵌静态资源 |
| 数据库 | SQLite (modernc.org/sqlite) | 纯 Go 驱动，可平滑切换 MySQL |
| Git 集成 | go-git | 纯 Go 实现，支持远程仓库自动初始化 |
| 加密 | AES-256-GCM | 数据源密码加密 |
| 路由 | Go 1.22 ServeMux | 原生 HTTP 路由，无框架 |

### 架构设计

```
┌─────────────────────────────────────────────┐
│              Web 层 (HTMX)                   │
│  登录 / 工作台 / 脚本 / 发布 / 执行 / 归档    │
├─────────────────────────────────────────────┤
│              业务层 (internal/)              │
│  auth / config / crypto / repo               │
├─────────────────────────────────────────────┤
│              执行引擎 (executor)              │
│  MySQL 执行器 / 连接测试                      │
├─────────────────────────────────────────────┤
│              Git 集成 (git)                   │
│  clone / commit / push / 空仓库初始化         │
├─────────────────────────────────────────────┤
│              数据层 (SQLite)                  │
│  10 张表 / 迁移自动执行                       │
└─────────────────────────────────────────────┘
```

## 使用指南

### 1. 初始化配置

首次启动后，使用默认账号 `admin` / `admin123` 登录。

### 2. 配置项目

进入 **系统配置 → 项目管理**，创建项目并设置项目编码（用于 Git 仓库目录）。

### 3. 配置环境与数据源

进入 **系统配置 → 环境配置**，创建开发/测试/生产环境。

进入 **系统配置 → 数据源**，为每个环境配置数据源，点击「测试连接」验证连通性。

### 4. 录入脚本

进入 **脚本管理 → 脚本列表**，点击「新建脚本」，填写标题、描述、提交人，粘贴 SQL 内容。保存后自动同步到 Git `pending/` 目录。

### 5. 发布编排

进入 **脚本管理 → 发布编排**，创建发布单，将脚本关联到发布。

### 6. 推送执行

在发布详情页选择目标数据源，勾选要执行的脚本，点击「推送执行」。执行成功后脚本状态自动更新为「已执行」。

### 7. 归档提交

进入 **归档版本 → 归档记录**，选择已执行的发布进行归档。归档时脚本写入 Git `archived/` 目录，版本号自动自增。

## 开发

### 从源码运行

```bash
go mod download
go run ./cmd/server/
```

### 编译

```bash
# 当前平台
go build -o sql-mgr-server ./cmd/server/

# 交叉编译
GOOS=linux GOARCH=amd64 go build -o sql-mgr-server-linux ./cmd/server/
GOOS=windows GOARCH=amd64 go build -o sql-mgr-server.exe ./cmd/server/
GOOS=darwin GOARCH=arm64 go build -o sql-mgr-server-macos ./cmd/server/
```

### 数据库表结构

| 表名 | 说明 |
|------|------|
| `project` | 项目 |
| `project_version` | 项目版本 |
| `environment` | 环境配置 |
| `datasource` | 数据源 |
| `sql_script` | SQL 脚本 |
| `release` | 发布单 |
| `release_script` | 发布-脚本关联 |
| `execution_record` | 执行记录（唯一数据源） |
| `archive_record` | 归档记录 |
| `settings` | 系统配置（Git 等） |

## Roadmap

- [ ] 支持 PostgreSQL 数据源
- [ ] 脚本审批流程
- [ ] 执行回滚
- [ ] 邮件/钉钉通知
- [ ] 多用户权限管理
- [ ] 脚本差异对比

## 贡献

欢迎提交 Issue 和 PR。

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 创建 Pull Request

## License

[MIT](LICENSE) © 2026 powerchen
