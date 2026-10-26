# Utopia Images

> 又一个 Utopia Images 自部署图床

Utopia Images 是一个现代、高性能且开箱即用的自托管图床与静态资源托管平台。后端采用 Go 语言构建，前端基于 Vue 3 + Naive UI + TypeScript 开发，支持编译打包为单一独立二进制文件，无需复杂的外部依赖环境即可轻松运行。

---

## 核心特性

- **单二进制独立部署**：前端静态资产全部通过 Go `embed` 嵌入二进制执行文件，下载即可直接运行，无需额外部署 Web 服务器或 Node.js 环境。
- **多数据库支持**：
  - 默认使用轻量级 SQLite 文件数据库，零配置开箱即用。
  - 大规模高并发企业部署推荐使用 PostgreSQL。
  - 数据库表结构完全遵循第三范式 (3NF) 设计，严谨清晰。
- **灵活存储策略与多挂载点**：
  - 支持多套物理存储策略，自定义隔离存储目录。
  - 用户本地存储默认上限为 10GB，管理员存储空间无上限。
  - 支持自定义外部访问 URL 路径前缀（如 `/uploads`），亦支持将策略直接挂载至站点顶级根路径（留空前缀）。
- **全功能富画布图片编辑器**：
  - **交互式自由裁剪**：默认优先裁切模式，支持选区居中自由拖拽及 8 方向边缘与角落控制手柄缩放。
  - **几何与涂鸦标注**：支持画笔、矩形、圆角矩形、圆形、箭头、文字印章、马赛克与局部模糊。
  - **旋转翻转与滤镜微调**：支持 90° 顺逆时针旋转、水平/垂直镜像翻转、黑白、复古、反色及亮度、对比度、饱和度、高斯模糊微调。
  - **版本更新与缓存穿透**：原图覆盖保存后自动清理派生图缓存，配合 ETag 与浏览器缓存验证机制，确保前台即时更新。
- **智能水印引擎**：
  - 支持单点九宫格定位与全图倾斜平铺两种水印布局。
  - 支持**自适应背景明暗对比色**，根据图片底色智能反转文字颜色，防止水印干扰画面。
  - 支持**动态变量标签**（如年份、日期、请求域名、图片体积、图片分辨率等），包含动态标签时自动跳过磁盘静态缓存并按需实时渲染。
  - 跨平台字体回退机制，确保在各种服务器环境下稳定呈现文字。
- **企业级多重安全防线**：
  - **现代双因素身份验证 (2FA)**：同时支持基于 TOTP 的时间同步验证码与基于 WebAuthn 的 **Passkey（指纹、面容或硬件安全密钥）** 无密码登录。
  - **防盗链保护 (Hotlink Protection)**：支持按全局、按用户及按文件独立配置 HTTP Referer 白名单，支持空 Referer 访问开关。
  - **临时免水印签名链接**：支持为指定图片生成具备时效性的免水印直链，吊销后即刻失效。
  - **邮箱激活与凭证找回**：用户注册流程支持全自动邮箱激活校验，确保只有合法邮箱用户方可启用。
- **角色化配额与多租户权限**：
  - 超级管理员角色默认空间无上限 (`0`)。
  - 支持自定义用户角色、每位用户的存储容量配额以及该角色允许绑定的物理存储策略。
- **批量归档包一键导入**：
  - 支持 `.zip`、`.tar`、`.gz`、`.tar.gz` (`.tgz`)、`.7z` 等多种归档压缩包直接上传导入。
  - 支持保留原有目录树层级结构或平铺入库，支持同名冲突跳过、覆盖或自动重命名。
- **开发者 API 与个人访问令牌 (PAT)**：
  - 提供开箱即用的 RESTful API 与永久/限期个人访问令牌。
  - 无缝兼容 PicGo、Typora、Obsidian、ShareX 等桌面客户端及第三方程序接入。

---

## 快速上手

### 方式一：直接运行预编译二进制文件

1. 从 [Releases 页面](https://github.com/UtopiaXC/UtopiaImages/releases) 下载适合您系统架构的二进制包。
2. 解压并在终端中启动：
   ```bash
   ./utopiaimages
   ```
3. 打开浏览器访问 `http://localhost:8080`，系统将自动进入可视化安装向导。配置站点名称、数据库连接、邮件发信服务（SMTP）及初始化超级管理员账号后即可进入系统。

### 方式二：Docker 部署

```bash
docker run -d \
  --name utopiaimages \
  -p 8080:8080 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/storage:/app/storage \
  utopiaxc/utopiaimages:latest
```

使用 Docker Compose：
```yaml
version: '3.8'

services:
  utopiaimages:
    image: utopiaxc/utopiaimages:latest
    container_name: utopiaimages
    restart: always
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/data
      - ./storage:/app/storage
      - ./config.yaml:/app/config.yaml
```

---

## 开发者与 API 文档

UtopiaImages 针对第三方接入提供了完善的 REST API 接口文档，详细内容请参阅：
[**API 个人访问令牌 (Personal Access Token) 文档**](assets/documentation/api_tokens.md)

### 常用 PicGo 配置说明
- **图床类型**：自定义 Web 图床
- **API 地址**：`https://your-domain.com/api/v1/upload`
- **POST 参数名**：`file`
- **JSON 路径**：`links.url`
- **自定义请求头**：`{"Authorization": "Bearer uimg_xxxxxxxxxxxxxxxx"}`

---

## 本地编译构建

### 环境要求
- **Go** >= 1.22
- **Node.js** >= 18.x
- **npm** 或 **pnpm**

### 1. 构建前端 SPA
```bash
cd web
npm install
npm run build
cd ..
```
构建产物将输出至 `web/dist`，并在后续步骤中被 Go 自动打包进最终可执行文件。

### 2. 构建后端单一可执行程序
```bash
# Windows
go build -o utopiaimages.exe .

# Linux / macOS
go build -o utopiaimages .
```

---

## 技术栈

- **后端**：[Go](https://go.dev/)、[Gin Web Framework](https://github.com/gin-gonic/gin)、[GORM](https://gorm.io/)、[Imaging](https://github.com/disintegration/imaging)、[WebAuthn](https://github.com/go-webauthn/webauthn)、[PuerkitoBio/goquery](https://github.com/PuerkitoBio/goquery)
- **前端**：[Vue 3](https://vuejs.org/)、[TypeScript](https://www.typescriptlang.org/)、[Vite](https://vitejs.dev/)、[Naive UI](https://www.naiveui.com/)、[Vue I18n](https://kazupon.github.io/vue-i18n/)、[Ionicons](https://ionic.io/ionicons)
- **多语言**：简体中文 (zh-CN)、英语 (en-US)、日语 (ja-JP)

---

## 开源许可证

本项目采用 [MIT License](LICENSE) 开源协议。


## AI使用
本文档与项目基本由Claude Opus 5.5与Gemini 3.8 Flash实现