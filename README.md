# Server 后端项目说明文档

这是一份面向开发、接手和维护的引导文档，用来说明当前 `server` 后端项目的整体结构、各模块职责、初始化步骤、常用命令和运行注意事项。

项目是一个基于 Go 的博客/内容站后端，核心能力包括用户登录注册、文章管理与搜索、评论、图片上传、站点配置、广告、友链、反馈、微信配置、AI 对话、RAG 文章知识库、定时任务和异步任务队列。

## 1. 技术栈概览

| 类型 | 技术/组件 | 用途 |
| --- | --- | --- |
| 后端语言 | Go | 项目主语言，模块名为 `server` |
| Web 框架 | Gin | HTTP API、路由、中间件 |
| ORM | GORM | MySQL 表结构迁移和关系数据读写 |
| 数据库 | MySQL | 用户、评论、图片、广告、友链、反馈、登录日志等关系型数据 |
| 搜索引擎 | Elasticsearch 8 | 文章主体索引、文章搜索、RAG 切片索引 |
| 缓存/队列 | Redis | 缓存热点数据、日历、浏览量增量，同时作为 Asynq 队列后端 |
| 异步任务 | Asynq | 后台执行 RAG 文章向量同步任务 |
| 定时任务 | robfig/cron | 定时同步浏览量、热点榜、日历、RAG 维护任务 |
| 日志 | zap + lumberjack | JSON 日志、文件轮转、可选控制台输出 |
| 鉴权 | JWT | 登录态、刷新令牌、管理员权限控制 |
| 上传 | 本地存储/七牛云 | 图片上传、图片资源管理 |
| AI/RAG | 通义千问兼容接口 + Embedding | 博客问答、文章召回、向量化切片 |

`go.mod` 中当前声明的 Go 版本是 `go 1.25.6`。本地开发时建议优先使用与项目声明一致或更高兼容版本的 Go。

## 2. 项目启动流程

入口文件是 `main.go`。启动时会按下面顺序初始化：

1. 读取 `config.yaml`，反序列化到 `config.Config`。
2. 初始化 zap 日志。
3. 初始化 JWT 黑名单本地缓存等其他全局状态。
4. 连接 MySQL，并创建 GORM 实例。
5. 连接 Redis。
6. 连接 Elasticsearch。
7. 创建 Asynq Client。
8. 检查命令行参数，如果带了初始化/维护 flag，就执行对应命令并退出。
9. 启动 Asynq Server，注册 RAG 异步 Worker。
10. 启动 cron 定时任务。
11. 初始化 Gin 路由并启动 HTTP 服务。

这意味着：即使只是执行 `-sql`、`-es`、`-admin` 这类一次性命令，也需要先保证 `config.yaml`、MySQL、Redis、Elasticsearch 等基础配置可用。

## 3. 目录结构说明

| 目录/文件 | 作用 |
| --- | --- |
| `main.go` | 项目入口，串联配置、日志、数据库、Redis、ES、Asynq、cron 和 HTTP 服务 |
| `config.yaml` | 项目运行配置，包含系统、数据库、Redis、ES、JWT、上传、邮箱、AI 等配置 |
| `go.mod` / `go.sum` | Go 依赖定义和锁定文件 |
| `mysql_20260327.sql` | 项目中已有的一份 MySQL 数据备份/初始化 SQL |
| `postMan.json` | Postman 接口集合，可导入 Postman 调试接口 |
| `core/` | 核心启动能力：读取配置、初始化日志、启动 HTTP 服务、优雅退出 |
| `config/` | `config.yaml` 对应的 Go 配置结构体 |
| `global/` | 全局对象：配置、日志、DB、Redis、ES、Asynq、JWT 黑名单缓存 |
| `initialize/` | 初始化模块：GORM、Redis、ES、Router、cron、Asynq 等 |
| `router/` | 路由注册层，定义每个业务模块挂载到哪些 API 路径 |
| `api/` | HTTP 控制器层，负责参数接收、校验、调用 service、返回统一响应 |
| `service/` | 业务逻辑层，处理文章、用户、评论、AI、上传、配置等核心逻辑 |
| `model/database/` | MySQL GORM 模型 |
| `model/elasticsearch/` | Elasticsearch 文档结构和索引 mapping |
| `model/request/` | API 请求参数结构 |
| `model/response/` | API 响应结构 |
| `model/appTypes/` | 业务枚举类型，例如用户角色、注册来源、图片分类、存储类型 |
| `model/other/` | 外部接口或通用数据结构 |
| `middleware/` | Gin 中间件：JWT 鉴权、管理员鉴权、登录记录、请求日志、异常恢复 |
| `flag/` | 命令行初始化和维护命令，例如建表、导入导出、建索引、创建管理员 |
| `task/` | cron 定时任务入口和任务实现 |
| `tasks/` | Asynq 异步任务定义 |
| `worker/` | Asynq Worker 任务处理逻辑 |
| `utils/` | 工具方法：JWT、邮箱、上传、分页、HTTP、热点榜、日历、图片等 |
| `uploads/` | 本地上传文件目录 |
| `log/` | 日志输出目录 |
| `.workflow/` | CI 构建配置，会构建 Linux、Windows、macOS 多平台产物 |

## 4. 分层关系

项目的主要调用链如下：

```text
HTTP 请求
  -> router 路由分组
  -> middleware 鉴权/日志/登录记录
  -> api 控制器
  -> service 业务逻辑
  -> global.DB / global.ESClient / global.Redis / global.AsynqClient
  -> model 数据结构
  -> model/response 统一响应
```

这种结构下，新增业务时一般按下面顺序处理：

1. 在 `model/request` 和 `model/response` 中定义请求/响应结构。
2. 在 `model/database` 或 `model/elasticsearch` 中定义存储结构。
3. 在 `service` 中实现业务逻辑。
4. 在 `api` 中处理 HTTP 参数和响应。
5. 在 `router` 中注册接口路径。
6. 如需初始化表或索引，补充 `flag` 或对应 mapping。

## 5. 核心模块说明

### 5.1 core 核心启动模块

`core/` 负责项目最底层的启动能力。

| 文件 | 说明 |
| --- | --- |
| `core/conf.go` | 读取 `config.yaml`，解析为全局配置 |
| `core/zap.go` | 初始化 zap 日志，支持日志文件轮转和控制台输出 |
| `core/server.go` | 启动 HTTP 服务，监听退出信号并执行优雅关闭 |
| `core/server_win.go` | Windows 平台 HTTP server 初始化 |
| `core/server_other.go` | 非 Windows 平台 HTTP server 初始化 |

HTTP 服务地址由 `config.yaml` 的 `system.host` 和 `system.port` 拼出来。当前配置中可看到默认端口为 `8080`，路由前缀为 `api`。

### 5.2 config 配置模块

`config/` 中的结构体对应 `config.yaml` 的配置项。

主要配置组如下：

| 配置组 | 作用 |
| --- | --- |
| `system` | 服务监听地址、端口、Gin 环境、API 前缀、会话密钥、存储类型 |
| `mysql` | MySQL 连接信息、连接池、GORM 日志模式 |
| `redis` | Redis 地址、密码、DB |
| `es` | Elasticsearch 地址、账号、密码、是否打印请求日志 |
| `jwt` | access token、refresh token 密钥和过期时间 |
| `upload` | 图片大小限制、本地上传路径 |
| `qiniu` | 七牛云空间、域名、AK/SK、HTTPS/CDN 配置 |
| `email` | SMTP 邮箱配置，用于验证码邮件 |
| `qq` | QQ 登录配置 |
| `wechat` | 微信 JS SDK 相关配置 |
| `gaode` | 高德 IP 定位和天气接口配置 |
| `website` | 站点标题、Logo、备案、个人资料、社交链接等 |
| `captcha` | 图形验证码尺寸、长度、干扰点等 |
| `ai` | AI 开关、模型、Embedding、RAG 索引、超时、召回数量、维护任务等 |
| `zap` | 日志级别、日志文件、保留策略、是否输出到控制台 |

注意：`config.yaml` 中包含数据库密码、JWT 密钥、第三方平台密钥等敏感信息。提交代码或分享文档时，不要把真实密钥写到文档、截图或公开仓库中。

### 5.3 initialize 初始化模块

`initialize/` 负责把各种外部资源连接起来。

| 文件 | 说明 |
| --- | --- |
| `initialize/gorm.go` | 初始化 GORM，连接 MySQL，设置连接池 |
| `initialize/redis.go` | 连接 Redis，并通过 `Ping` 检查可用性 |
| `initialize/es.go` | 初始化 Elasticsearch Typed Client |
| `initialize/router.go` | 初始化 Gin、全局中间件、静态文件、业务路由 |
| `initialize/cron.go` | 创建 cron 实例并注册定时任务 |
| `initialize/asynq.go` | 创建 Asynq Client/Server，注册 RAG Worker |
| `initialize/other.go` | 初始化 JWT 黑名单本地缓存等辅助状态 |

如果 MySQL、Redis、Elasticsearch 连接失败，项目会在启动阶段报错或退出，所以初始化前需要先确认依赖服务已经启动。

### 5.4 router 路由模块

`router/` 只负责注册接口，不写具体业务。所有 API 都会挂在 `config.yaml` 的 `system.router_prefix` 下。当前配置中该值为 `api`，因此常见接口形如：

```text
http://127.0.0.1:8080/api/article/search
http://127.0.0.1:8080/api/user/login
```

路由按权限分为三类：

| 路由组 | 中间件 | 说明 |
| --- | --- | --- |
| publicGroup | 无登录要求 | 公开接口，例如文章搜索、站点信息、登录注册 |
| privateGroup | `JWTAuth()` | 登录用户接口，例如用户信息、评论、点赞 |
| adminGroup | `JWTAuth()` + `AdminAuth()` | 管理员接口，例如文章 CRUD、配置修改、图片管理 |

此外，`/` 和 `/article/:id` 是分享页入口，不走 `/api` 前缀，用于给首页和文章页注入 SEO/OpenGraph meta 信息。

### 5.5 api 控制器模块

`api/` 是 HTTP 控制器层，主要职责：

1. 从 Gin Context 中读取参数、Body、Query、Path。
2. 调用 `service` 层处理业务。
3. 用 `model/response` 返回统一结构。
4. 处理错误提示和状态码。

业务控制器包括：

| 文件 | 说明 |
| --- | --- |
| `api/base.go` | 验证码、邮箱验证码、QQ 登录地址 |
| `api/user.go` | 用户注册、登录、退出、资料、密码、天气、统计、后台用户管理 |
| `api/article.go` | 文章详情、搜索、分类、标签、点赞、后台 CRUD、RAG 同步管理 |
| `api/article_share.go` | 首页/文章分享页 meta 注入 |
| `api/comment.go` | 评论列表、最新评论、创建、删除、后台查询 |
| `api/image.go` | 图片上传、删除、列表 |
| `api/ai.go` | AI 对话、流式对话、快捷回复、模型管理 |
| `api/website.go` | 站点 Logo、标题、轮播、动态、日历、页脚链接 |
| `api/config.go` | 后台读取/修改配置 |
| `api/adverisement.go` | 广告信息、后台广告 CRUD |
| `api/friend_link.go` | 友链信息、后台友链 CRUD |
| `api/feedback.go` | 反馈提交、查询、回复、后台管理 |
| `api/wechat.go` | 微信 JS SDK config |

### 5.6 service 业务模块

`service/` 是项目的业务核心。控制器尽量只处理 HTTP，真正的数据读写和业务规则放在这里。

| 模块 | 说明 |
| --- | --- |
| `BaseService` | 发送邮箱验证码等基础能力 |
| `UserService` | 注册、登录、用户资料、冻结/解冻、登录记录、用户统计 |
| `JwtService` | Redis 中的 JWT 管理、黑名单管理 |
| `ArticleService` | 文章 ES 索引 CRUD、搜索、分类标签统计、点赞、RAG 同步状态 |
| `AIService` | 模型列表、模型切换、普通/流式聊天、RAG 召回、文章引用构建 |
| `RAGService` | RAG 切片、Embedding、向量索引创建、文章入库、维护扫描 |
| `CommentService` | 评论树查询、创建、删除、后台评论列表 |
| `ImageService` | 图片上传、删除、分页查询，支持本地或七牛 |
| `AdvertisementService` | 广告展示和后台 CRUD |
| `FriendLinkService` | 友链展示和后台 CRUD |
| `FeedbackService` | 用户反馈、后台回复和管理 |
| `WebsiteService` | 站点基础信息、轮播、新闻、日历、页脚链接 |
| `ConfigService` | 修改配置并写回 `config.yaml` |
| `EsService` | ES 索引创建、删除、存在性检查 |
| `GaodeService` | IP 定位、天气查询 |
| `QQService` | QQ OAuth access token 和用户信息获取 |
| `WechatService` | 微信 JS SDK 签名配置 |
| `HotSearchService` | 热搜数据读取 |
| `CalendarService` | 日历/节气数据读取 |

### 5.7 model 数据模型模块

`model/database/` 是 MySQL 表模型，主要包括：

| 模型 | 说明 |
| --- | --- |
| `User` | 用户账号、邮箱、头像、角色、注册来源、冻结状态 |
| `Login` | 登录日志，记录 IP、地址、系统、设备、浏览器、登录状态 |
| `JwtBlacklist` | JWT 黑名单 |
| `Image` | 图片资源，记录 URL、分类、存储类型 |
| `Comment` | 评论，支持父子评论树 |
| `Feedback` | 用户反馈和管理员回复 |
| `FriendLink` | 友情链接 |
| `FooterLink` | 页脚链接 |
| `Advertisement` | 广告内容 |
| `ArticleLike` | 用户文章点赞/收藏记录 |
| `ArticleCategory` | 文章分类及数量统计 |
| `ArticleTag` | 文章标签及数量统计 |

`model/elasticsearch/` 是 ES 文档模型：

| 模型 | 说明 |
| --- | --- |
| `Article` | 文章主体，包含标题、分类、标签、正文、浏览量、评论数、点赞数、RAG 同步状态 |
| `RagChunk` | RAG 文章切片，包含文章 ID、文本、向量、标题、URL、语言 |

当前项目的文章主体存储在 ES 的 `article_index` 中，而不是 MySQL 的文章表中。评论、点赞等关系数据仍由 MySQL 维护，并通过钩子或 service 同步计数到 ES 文章文档。

### 5.8 middleware 中间件模块

| 文件 | 说明 |
| --- | --- |
| `middleware/jwt.go` | 校验 access token、解析用户身份 |
| `middleware/admin.go` | 校验用户是否管理员 |
| `middleware/login_record.go` | 登录时记录 IP、地址、设备、浏览器等信息 |
| `middleware/logger.go` | 记录请求日志和 panic recovery |

鉴权相关工具集中在 `utils/jwt.go`、`utils/claims.go`、`service/jwt.go`。

### 5.9 flag 命令行模块

`flag/` 提供一次性初始化和维护命令。命令是在 `main.go` 启动早期执行的，执行完成后会退出，不会继续启动 HTTP 服务。

一次只能传一个 flag。如果同时传多个，程序会报错。

| 命令 | 说明 |
| --- | --- |
| `go run main.go -sql` | 使用 GORM AutoMigrate 初始化/更新 MySQL 表结构 |
| `go run main.go -sql-import mysql_20260327.sql` | 从 SQL 文件导入 MySQL 数据 |
| `go run main.go -sql-export` | 导出 MySQL 数据到 `mysql_日期.sql` |
| `go run main.go -es` | 创建文章 ES 索引 `article_index` |
| `go run main.go -es-import es_xxx.json` | 从 JSON 文件导入 ES 文章索引 |
| `go run main.go -es-export` | 导出 ES 文章索引到 `es_日期.json` |
| `go run main.go -rag-index` | 创建 RAG 切片索引 |
| `go run main.go -rag-ingest` | 将已有文章切片、向量化并写入 RAG 索引 |
| `go run main.go -admin` | 交互式创建管理员账号 |

`-sql-export` 当前通过 `docker exec mysql mysqldump ...` 执行，因此要求 MySQL 容器名是 `mysql`。如果你的 MySQL 不在这个容器中，需要调整 `flag/sql_export.go` 或手动执行 `mysqldump`。

### 5.10 task/tasks/worker 后台任务模块

项目有两类后台任务：cron 定时任务和 Asynq 异步任务。

cron 定时任务在 `task/` 中注册：

| 任务 | 频率 | 说明 |
| --- | --- | --- |
| `UpdateArticleViewsSyncTask` | `@hourly` | 将 Redis 中缓存的文章浏览量增量同步到 ES |
| `GetHotListSyncTask` | `@hourly` | 拉取百度、知乎、快手、头条热搜，缓存到 Redis |
| `GetCalendarSyncTask` | `@daily` | 拉取日历/节气数据，缓存到 Redis |
| `MaintainRAGSyncTask` | 配置控制 | 扫描文章 RAG 状态，把需要同步的文章投递到 Asynq |

RAG 维护任务由 `config.yaml` 中的 `ai.rag_maintenance_enable` 开关控制，执行频率由 `ai.rag_maintenance_spec` 控制。为空时默认使用 `@every 30m`。

Asynq 异步任务在 `tasks/` 和 `worker/` 中定义：

| 文件 | 说明 |
| --- | --- |
| `tasks/rag.go` | 定义 `rag:sync` 任务类型、`rag` 队列和任务 payload |
| `worker/rag.go` | 处理单篇文章的 RAG 同步：读取文章、切片、Embedding、写入索引、更新同步状态 |

## 6. 主要接口模块导航

下面是按模块整理的主要接口路径。实际路径会带上 `system.router_prefix`，当前配置是 `api`，因此完整路径通常是 `/api/...`。

### 6.1 基础接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/base/captcha` | 获取图形验证码 |
| `POST` | `/api/base/sendEmailVerificationCode` | 发送邮箱验证码 |
| `GET` | `/api/base/qqLoginURL` | 获取 QQ 登录跳转地址 |

### 6.2 用户接口

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| `POST` | `/api/user/register` | 公开 | 用户注册 |
| `POST` | `/api/user/login` | 公开 | 用户登录 |
| `POST` | `/api/user/forgotPassword` | 公开 | 忘记密码 |
| `GET` | `/api/user/card` | 公开 | 获取用户卡片信息 |
| `POST` | `/api/user/logout` | 登录 | 退出登录 |
| `PUT` | `/api/user/resetPassword` | 登录 | 修改密码 |
| `GET` | `/api/user/info` | 登录 | 当前用户信息 |
| `PUT` | `/api/user/changeInfo` | 登录 | 修改用户资料 |
| `GET` | `/api/user/weather` | 登录 | 查询天气 |
| `GET` | `/api/user/chart` | 登录 | 用户图表统计 |
| `GET` | `/api/user/list` | 管理员 | 用户列表 |
| `PUT` | `/api/user/freeze` | 管理员 | 冻结用户 |
| `PUT` | `/api/user/unfreeze` | 管理员 | 解冻用户 |
| `GET` | `/api/user/loginList` | 管理员 | 登录记录 |

### 6.3 文章接口

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/api/article/:id` | 公开 | 文章详情 |
| `GET` | `/api/article/search` | 公开 | 文章搜索 |
| `GET` | `/api/article/category` | 公开 | 文章分类 |
| `GET` | `/api/article/tags` | 公开 | 文章标签 |
| `POST` | `/api/article/like` | 登录 | 点赞/收藏文章 |
| `GET` | `/api/article/isLike` | 登录 | 判断是否已点赞 |
| `GET` | `/api/article/likesList` | 登录 | 用户点赞列表 |
| `POST` | `/api/article/create` | 管理员 | 创建文章 |
| `DELETE` | `/api/article/delete` | 管理员 | 删除文章 |
| `PUT` | `/api/article/update` | 管理员 | 更新文章 |
| `GET` | `/api/article/list` | 管理员 | 后台文章列表 |
| `POST` | `/api/article/sync/:id` | 管理员 | 重试单篇文章 RAG 同步 |
| `DELETE` | `/api/article/sync/:id` | 管理员 | 清理单篇文章 RAG 同步状态 |
| `POST` | `/api/admin/articles/sync/:id` | 管理员 | 新版 RAG 同步重试路径 |
| `DELETE` | `/api/admin/articles/sync/:id` | 管理员 | 新版 RAG 同步清理路径 |

### 6.4 AI/RAG 接口

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| `POST` | `/api/ai/chat` | 公开 | AI 问答 |
| `POST` | `/api/ai/chat/stream` | 公开 | AI 流式问答 |
| `GET` | `/api/ai/quick-replies` | 公开 | 快捷回复 |
| `PUT` | `/api/ai/current-model` | 公开 | 修改前台当前模型 |
| `POST` | `/api/chat` | 公开 | 兼容路径：AI 问答 |
| `POST` | `/api/chat/stream` | 公开 | 兼容路径：AI 流式问答 |
| `GET` | `/api/chat/quick-replies` | 公开 | 兼容路径：快捷回复 |
| `GET` | `/api/chat/models` | 公开 | 前台可见模型列表 |
| `GET` | `/api/ai/models` | 管理员 | 模型列表 |
| `POST` | `/api/ai/models` | 管理员 | 新增模型 |
| `POST` | `/api/ai/models/bulk` | 管理员 | 批量新增模型 |
| `PUT` | `/api/ai/models` | 管理员 | 更新模型 |
| `DELETE` | `/api/ai/models` | 管理员 | 删除模型 |
| `PUT` | `/api/ai/models/current` | 管理员 | 设置当前模型 |

### 6.5 评论接口

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/api/comment/:article_id` | 公开 | 查询某篇文章评论 |
| `GET` | `/api/comment/new` | 公开 | 最新评论 |
| `POST` | `/api/comment/create` | 登录 | 创建评论 |
| `DELETE` | `/api/comment/delete` | 登录 | 删除评论 |
| `GET` | `/api/comment/info` | 登录 | 当前用户评论 |
| `GET` | `/api/comment/list` | 管理员 | 后台评论列表 |

### 6.6 图片接口

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| `POST` | `/api/image/upload` | 管理员 | 上传图片 |
| `DELETE` | `/api/image/delete` | 管理员 | 删除图片 |
| `GET` | `/api/image/list` | 管理员 | 图片列表 |

### 6.7 站点与展示接口

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/api/website/logo` | 公开 | 站点 Logo |
| `GET` | `/api/website/title` | 公开 | 站点标题 |
| `GET` | `/api/website/info` | 公开 | 站点基础信息 |
| `GET` | `/api/website/carousel` | 公开 | 首页轮播 |
| `GET` | `/api/website/news` | 公开 | 热搜/新闻 |
| `GET` | `/api/website/calendar` | 公开 | 日历信息 |
| `GET` | `/api/website/footerLink` | 公开 | 页脚链接 |
| `POST` | `/api/website/addCarousel` | 管理员 | 添加轮播 |
| `PUT` | `/api/website/cancelCarousel` | 管理员 | 取消轮播 |
| `POST` | `/api/website/createFooterLink` | 管理员 | 创建页脚链接 |
| `DELETE` | `/api/website/deleteFooterLink` | 管理员 | 删除页脚链接 |

### 6.8 其他业务接口

| 模块 | 主要路径 | 说明 |
| --- | --- | --- |
| 广告 | `/api/advertisement/...` | 公开获取广告信息，管理员进行广告 CRUD |
| 友链 | `/api/friendLink/...` | 公开获取友链信息，管理员进行友链 CRUD |
| 反馈 | `/api/feedback/...` | 用户提交反馈，管理员回复和管理 |
| 配置 | `/api/config/...` | 管理员读取/修改网站、系统、邮箱、QQ、七牛、JWT、高德、AI 配置 |
| 微信 | `/api/wechat/js-config` | 获取微信 JS SDK 配置 |

## 7. 本地初始化步骤

下面是一套推荐的首次启动顺序。

### 7.1 准备依赖服务

本项目启动前需要准备：

1. MySQL
2. Redis
3. Elasticsearch 8
4. 可选：七牛云、SMTP 邮箱、QQ 互联、高德、微信、通义千问/Embedding API

MySQL 数据库需要先创建好，GORM 的 `AutoMigrate` 只会建表，不会自动创建数据库。

示例 SQL：

```sql
CREATE DATABASE your_database_name CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

然后把 `config.yaml` 中的 `mysql.db_name`、`mysql.username`、`mysql.password` 等配置改成自己的环境。

### 7.2 安装 Go 依赖

在项目根目录执行：

```bash
go mod download
```

### 7.3 检查核心配置

至少需要确认这些配置：

```yaml
system:
  host: 0.0.0.0
  port: 8080
  env: release
  router_prefix: api
  oss_type: local 或 qiniu

mysql:
  host: 127.0.0.1
  port: 3306
  db_name: your_database_name
  username: your_username
  password: your_password

redis:
  address: 127.0.0.1:6379
  password: ""
  db: 0

es:
  url: http://127.0.0.1:9200
  username: your_es_username
  password: your_es_password

jwt:
  access_token_secret: 请换成自己的随机密钥
  refresh_token_secret: 请换成自己的随机密钥

upload:
  size: 5
  path: uploads
```

如果启用 AI/RAG，还需要配置：

```yaml
ai:
  enable: true
  qwen_api_key: your_api_key
  qwen_base_url: your_base_url
  qwen_model: your_chat_model
  qwen_embedding_model: your_embedding_model
  embedding_dimensions: 1024
  rag_index: rag_chunks
  article_base_url: https://your-site.com/article/
```

### 7.4 初始化 MySQL 表结构

如果是新数据库，可以先执行：

```bash
go run main.go -sql
```

这会根据 `model/database` 中的 GORM 模型创建或更新表结构。

如果你想导入项目已有 SQL 数据，可以执行：

```bash
go run main.go -sql-import mysql_20260327.sql
```

### 7.5 初始化 Elasticsearch 文章索引

```bash
go run main.go -es
```

该命令会创建 `article_index`。如果索引已存在，程序会交互式询问是否删除后重建。

如果你有 ES 导出的 JSON 文件，可以执行：

```bash
go run main.go -es-import es_20260327.json
```

文件名按实际情况替换。

### 7.6 初始化 RAG 索引

如果需要 AI 基于文章内容问答，需要创建 RAG 切片索引：

```bash
go run main.go -rag-index
```

如果已经有文章数据，并且 AI/Embedding 配置可用，可以把现有文章全量写入 RAG 索引：

```bash
go run main.go -rag-ingest
```

注意：`-rag-ingest` 会调用 Embedding API，可能产生接口调用费用，并且需要 `article_index` 中已经有文章数据。

### 7.7 创建管理员账号

```bash
go run main.go -admin
```

程序会提示输入邮箱和密码。密码要求长度 8 到 20 位。管理员的用户名和地址会从 `config.yaml` 的 `website.name`、`website.address` 中读取。

### 7.8 启动后端服务

```bash
go run main.go
```

当前配置下，服务地址通常是：

```text
http://127.0.0.1:8080
```

API 前缀通常是：

```text
/api
```

可以用下面命令简单验证：

```bash
curl http://127.0.0.1:8080/api/website/title
```

Windows PowerShell 中如果 `curl` 被别名覆盖，可以使用：

```powershell
curl.exe http://127.0.0.1:8080/api/website/title
```

## 8. 常用维护命令

### 8.1 构建二进制

普通构建：

```bash
go build -o main main.go
```

Windows 构建：

```powershell
go build -o main.exe main.go
```

`.workflow/` 中也提供了多平台构建示例：

```bash
GOOS=linux GOARCH=amd64 go build -o output/main.amd64 main.go
GOOS=windows GOARCH=amd64 go build -o output/main.win64.exe main.go
GOOS=darwin GOARCH=amd64 go build -o output/main.darwin main.go
```

### 8.2 导出 MySQL

```bash
go run main.go -sql-export
```

导出文件名格式为：

```text
mysql_YYYYMMDD.sql
```

注意：当前实现依赖 Docker 容器名 `mysql`。

### 8.3 导出 Elasticsearch

```bash
go run main.go -es-export
```

导出文件名格式为：

```text
es_YYYYMMDD.json
```

### 8.4 导入 Elasticsearch

```bash
go run main.go -es-import es_YYYYMMDD.json
```

导入时会重建 `article_index`，请谨慎操作。

### 8.5 手动维护 RAG

创建 RAG 索引：

```bash
go run main.go -rag-index
```

全量重建 RAG 内容：

```bash
go run main.go -rag-ingest
```

单篇文章的 RAG 重试/清理可以通过后台接口完成：

```text
POST   /api/article/sync/:id
DELETE /api/article/sync/:id
POST   /api/admin/articles/sync/:id
DELETE /api/admin/articles/sync/:id
```

## 9. 运行时数据说明

### 9.1 MySQL

MySQL 主要负责关系型数据：

1. 用户、角色、注册来源、冻结状态。
2. 登录日志和 JWT 黑名单。
3. 图片、广告、友链、页脚链接。
4. 评论、反馈。
5. 文章分类、标签和点赞记录。

### 9.2 Elasticsearch

Elasticsearch 主要负责文章内容和检索：

1. `article_index`：文章主体索引。
2. `rag_chunks` 或 `ai.rag_index` 配置的索引：RAG 切片向量索引。

文章创建、更新、删除主要围绕 ES 操作。文章分类、标签、点赞等辅助统计由 MySQL 配合维护。

### 9.3 Redis

Redis 当前主要用于：

1. 登录态和 JWT 相关缓存。
2. 文章浏览量临时累加。
3. 热搜数据缓存。
4. 日历数据缓存。
5. Asynq 异步任务队列。

### 9.4 文件系统

本地上传文件默认放在 `uploads/`。日志默认写入 `log/` 下，具体文件名由 `config.yaml` 的 `zap.filename` 控制。

如果上传配置使用七牛云，则图片会上传到七牛，同时数据库仍记录图片 URL 和存储类型。

## 10. 开发一个新模块的建议流程

假设要新增一个“专题”模块，可以按下面方式推进：

1. 在 `model/database` 中新增 `Topic` 表结构。
2. 在 `model/request` 中新增创建、更新、删除、列表查询参数。
3. 在 `model/response` 中新增需要返回给前端的数据结构。
4. 在 `service` 中新增 `TopicService`，实现业务逻辑。
5. 在 `service/enter.go` 的 `ServiceGroup` 中加入 `TopicService`。
6. 在 `api` 中新增 `topic.go`，调用 service 并返回统一响应。
7. 在 `api/enter.go` 中挂载对应 Api。
8. 在 `router` 中新增 `topic.go`，注册公开、登录或管理员路由。
9. 在 `router/enter.go` 中加入 `TopicRouter`。
10. 在 `flag/sql.go` 的 `AutoMigrate` 中加入新表。
11. 执行 `go run main.go -sql` 更新表结构。

如果新模块需要搜索或向量化能力，还要补充 `model/elasticsearch` 的文档结构和索引 mapping。

## 11. 接口调试

项目根目录有 `postMan.json`，可以导入 Postman 使用。

调试建议：

1. 先调用公开接口，例如 `/api/website/title`，确认服务可访问。
2. 调用 `/api/user/login` 登录，拿到 access token。
3. 调用登录接口时注意 cookie/refresh token 的处理。
4. 访问需要登录的接口时，在请求头中携带 token。
5. 访问后台接口时，登录用户必须是管理员角色。

统一响应结构在 `model/response/response.go` 中定义，常见返回包含：

```json
{
  "code": 0,
  "data": {},
  "msg": "ok"
}
```

具体 code 和 message 以实际接口返回为准。

## 12. 常见问题

### 12.1 启动时报 MySQL 连接失败

检查：

1. MySQL 服务是否启动。
2. `config.yaml` 中 host、port、username、password、db_name 是否正确。
3. 数据库是否已经创建。
4. 用户是否有该库的访问权限。

### 12.2 启动时报 Redis 连接失败

检查：

1. Redis 服务是否启动。
2. `redis.address` 是否正确。
3. 密码和 DB 是否正确。
4. 本地防火墙或 Docker 端口映射是否可访问。

### 12.3 启动时报 Elasticsearch 连接失败

检查：

1. ES 服务是否启动。
2. `es.url` 是否正确。
3. 如果 ES 开启安全认证，username/password 是否正确。
4. ES 版本是否兼容当前 `go-elasticsearch/v8`。

### 12.4 AI 对话没有返回或 RAG 没有内容

检查：

1. `ai.enable` 是否开启。
2. `qwen_api_key`、`qwen_base_url`、`qwen_model` 是否正确。
3. `qwen_embedding_model` 和 `embedding_dimensions` 是否匹配。
4. 是否已执行 `go run main.go -rag-index`。
5. 是否已执行 `go run main.go -rag-ingest` 或后台是否成功触发单篇文章同步。
6. `article_index` 中是否已有文章内容。

### 12.5 分享页报 `index.html not found`

`api/article_share.go` 会尝试从下面位置寻找前端入口文件：

```text
../web/dist/index.html
web/dist/index.html
dist/index.html
../web/index.html
web/index.html
```

如果部署时需要首页和文章分享页正常注入 meta，请确保前端构建产物放在上述路径之一，或调整 `loadIndexHTML()` 的候选路径。

### 12.6 上传图片失败

检查：

1. `upload.path` 目录是否存在且可写。
2. `upload.size` 是否小于上传文件大小。
3. `system.oss_type` 是 `local` 还是 `qiniu`。
4. 如果使用七牛，检查 `qiniu` 相关配置和 bucket 权限。

### 12.7 管理员接口返回无权限

检查：

1. 请求是否携带有效 access token。
2. 用户是否已登录且未被冻结。
3. 用户 `role_id` 是否为管理员角色。
4. token 是否进入 JWT 黑名单。

## 13. 推荐的首次启动命令清单

按顺序执行：

```bash
go mod download
go run main.go -sql
go run main.go -es
go run main.go -rag-index
go run main.go -admin
go run main.go
```

如果需要导入已有数据，可以替换或补充：

```bash
go run main.go -sql-import mysql_20260327.sql
go run main.go -es-import es_YYYYMMDD.json
go run main.go -rag-ingest
```

最后访问：

```text
http://127.0.0.1:8080
http://127.0.0.1:8080/api/website/title
```

到这里，后端项目的主要初始化、启动和维护流程就基本走通了。
