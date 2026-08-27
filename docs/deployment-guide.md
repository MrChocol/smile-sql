# SQL 统一管理平台 — 部署指南

## 构建方式

### 在线环境构建（有网络）

```bash
cd sql-mgr
go build -o sql-mgr ./cmd/server
```

### 离线环境构建（内网部署）

在有网络的机器上预先下载依赖：
```bash
cd sql-mgr
go mod vendor
```

将整个项目目录（含 vendor/）拷贝到内网服务器后构建：
```bash
cd sql-mgr
go build -o sql-mgr ./cmd/server
```

构建产物为单个二进制文件 `sql-mgr`，无外部依赖。

## 配置

创建 `config.yaml`（与二进制同目录）：

```yaml
server_port: 8080
sqlite_path: sql-mgr.db
git_repo_url: ""              # Git 仓库地址，留空禁用归档
git_token_enc: ""             # Git 访问令牌
encrypt_key: "your-32-byte-key!!"  # AES-256 加密密钥
admin_pw_hash: ""             # bcrypt 哈希，用于查看密码二次验证
```

也支持环境变量覆盖：`SQLMGR_SERVER_PORT`, `SQLMGR_SQLITE_PATH`, `SQLMGR_GIT_REPO_URL`, `SQLMGR_GIT_TOKEN_ENC`, `SQLMGR_ENCRYPT_KEY`, `SQLMGR_ADMIN_PW_HASH`

## 启动

```bash
./sql-mgr
```

首次启动自动创建 SQLite 数据库和默认管理员账户 `admin/admin123`。

## 管理密码生成

用于数据源密码查看的二次验证，生成 bcrypt 哈希：

```bash
htpasswd -bnBC 10 "" "your-management-password" | tr -d ':\n' | sed 's/$2y/$2a/'
```

将输出填入 `config.yaml` 的 `admin_pw_hash` 字段。

## 目录结构

```
sql-mgr/
├── cmd/server/main.go        # 入口
├── internal/
│   ├── archive/              # 归档引擎（Git 提交 + 版本自增）
│   ├── config/               # 配置加载
│   ├── crypto/               # AES-256-GCM 加密 + bcrypt 验证
│   ├── db/                   # SQLite 连接 + 迁移
│   ├── executor/             # SQL 执行引擎（MySQL）
│   ├── git/                  # Git 集成（go-git）
│   ├── model/                # 数据模型
│   ├── repo/                 # 仓储层（CRUD）
│   └── web/                 # Web 层（HTTP handlers + 模板）
├── migrations/               # DDL
├── vendor/                   # 离线依赖（go mod vendor）
├── web/templates/            # HTML 模板
├── config.yaml               # 配置文件
└── docs/                     # 文档
```
