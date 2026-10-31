# Utopia Images

> Yet Another Self Hosted Image Hosting System By GoLang.

Utopia Images是一个自托管图床系统，集成了图片编辑、水印、多存储策略、多语言、批量导入与路径映射等特性，并且除了图片外，也支持上传其他类型的文件比如CSS与JS。使用相对高效快速的Go语言开发。  

Demo图请见最底部。  

---

## 演示 DEMO

> 注：该功能不太好用，暂不推荐使用。我正在尝试用更方便的办法来实现。  
> 另外本演示使用的源码为最新源码，可能与docker发行版不一致。  

如果希望在线尝试，可以点击下方徽章打开GitHub Codespaces免费尝试（GitHub每个月为每名用户提供120核时免费），需要登录GitHub账号，默认端口2983。  

[![Open in GitHub Codespaces](https://github.com/codespaces/badge.svg)](https://codespaces.new/UtopiaXC/UtopiaImages)

---

## 快速上手

注：在任何情况下，任何软件我都推荐使用隔离环境内部署。尽可能使用独立虚拟机或rootless docker，而不是直接在物理机上运行任何程序。  
无论是人还是AI的代码都会出现莫名其妙的漏洞，鉴权问题、路径穿越、提权漏洞比比皆是，一定要多加防范。  

### 方式一：直接运行预编译二进制文件

1. 从 [Releases 页面](https://github.com/UtopiaXC/UtopiaImages/releases) 下载适合您系统架构的压缩包。
2. 解压并在终端中启动：
   ```bash
   ./utopia_images
   ```
3. 打开浏览器访问 `http://localhost:2983`，系统将进入可视化安装向导。配置站点名称、数据库连接、邮件发信服务（SMTP）及初始化超级管理员账号后即可进入系统。

### 方式二：Docker 部署

#### 1. 使用预编译镜像运行

系统核心数据统一保存在 `/app/data` 目录下（包含配置文件、数据库与原始图片存储）。只要挂载 `/app/data` 即可完整保证数据安全与整站迁移。  
`logs`（服务日志）与 `cache`（派生图片缩略图缓存）与 `data` 同级，属于可选项：若不挂载，则直接写在容器内部；若希望在宿主机留存日志或缩略图缓存，可按需添加映射。

基础运行（推荐，仅挂载核心数据）：
```bash
docker run -d \
  --name utopia_images \
  -p 2983:2983 \
  -v $(pwd)/data:/app/data \
  utopiaxc/utopia_images:latest
```

包含可选目录的完整挂载运行：
```bash
docker run -d \
  --name utopia_images \
  -p 2983:2983 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/logs:/app/logs \
  -v $(pwd)/cache:/app/cache \
  utopiaxc/utopia_images:latest
```

使用 Docker Compose：

```yaml
version: '3.8'

services:
  utopia_images:
    image: utopiaxc/utopia_images:latest
    container_name: utopia_images
    restart: always
    ports:
      - "2983:2983"
    volumes:
      - ./data:/app/data
      # 可选挂载：如需在宿主机留存日志或缩略图缓存，可取消以下注释
      # - ./logs:/app/logs
      # - ./cache:/app/cache
    environment:
      - TZ=Asia/Shanghai
```

挂载目录说明：
- `/app/data`（必需）：包含 `config/`（配置文件 `config.yaml`）、`db/`（数据库文件）与 `storage/`（原始图片存储）。日常备份只需备份此目录即可完整迁移整站数据与核心配置。
- `/app/logs`（可选）：服务运行日志（基于 lumberjack 自动轮转）。不挂载时自动保存在容器内。
- `/app/cache`（可选）：派生图片缩略图及处理缓存。不挂载时自动保存在容器内。

#### 2. 自行编译 Docker 镜像

```bash
git clone https://github.com/UtopiaXC/UtopiaImages.git
cd UtopiaImages

docker build -t utopia_images:latest -f deploy/Dockerfile .

docker run -d \
  --name utopia_images \
  -p 2983:2983 \
  -v $(pwd)/data:/app/data \
  utopia_images:latest
```

---

## 开发背景与进度

本人此前使用兰空图床，虽然开源版已经停止维护，但是勉强能用，所以一直没换。直到本次严重PHP漏洞，不得不将php版本更新到最新，然而兰空图床的安装和依赖极其复杂，在我更新PHP后就无法正常启动了。鉴于其复杂、反应迟钝、不再维护甚至基于PHP的现状，因此我决定彻底抛弃它转向新平台。  

因此我产生了开发一个Go版本的图床的想法。要能兼容兰空甚至其他图床的路径，要使用docker一键部署，要支持水印、防盗链、多策略等特性。
### AI使用

本项目使用Claude Opus 5.5完成需求分析与可行性验证，使用Gemini 3.8 Flash进行开发，并使用Gemini 4 Argon进行漏洞检测。  

### 开发计划与目前支持情况

由于我暂时没有对象存储的需求，因此第一版只添加了本地存储策略的支持。  

未来我计划添加对阿里云OSS、腾讯云COS、Amazon S3、GCP CGS的支持。如果有需要或者希望添加其他对象存储的支持，可以提交Issue或PR。  

非登录游客上传功能是否要开发我还在思考，如果有需要可以提交Issue。  

---

## 技术栈

- 后端：Go、Gin Web Framework、GORM
- 前端：Vue 3、TypeScript、Vite、Naive UI、Vue I18n、Ionicons
- 多语言：简体中文、英语、日语（其他语言支持可以提交issue或pr）
---

## 开发者与 API 文档

针对第三方接入提供了REST API接口文档，详细内容请参阅：
[API 个人访问令牌 (Personal Access Token) 文档](assets/documentation/api_tokens.md)

### 常用PicGo配置说明
- 图床类型：自定义 Web 图床
- API 地址：`https://your-domain.com/api/v1/upload`
- POST 参数名：`file`
- JSON 路径：`links.url`
- 自定义请求头：`{"Authorization": "Bearer uimg_xxxxxxxxxxxxxxxx"}`

---

## 本地编译构建

### 环境要求
- Go >= 1.22
- Node.js >= 18.x
- npm 或 pnpm

### 1. 构建前端
```bash
cd web
npm install
npm run build
cd ..
```
构建产物输出至 `web/dist`，后续步骤中由Go embed自动打包进最终可执行文件。

### 2. 构建后端单一可执行程序
```bash
# Windows
go build -ldflags="-s -w" -o utopia_images.exe ./cmd/utopia_images/main.go

# Linux / macOS
go build -ldflags="-s -w" -o utopia_images ./cmd/utopia_images/main.go
```

---

## 社区协作

本项目欢迎贡献代码。  
欢迎AI代码。但是提交AI代码前请务必人工审阅。  
请遵守社区规则提交issue与PR。

---

## 捐助

请不要在任何渠道以任何方式为本项目付出金钱。  
如果您想捐助本项目，您可以向慈善组织或开放源代码促进会（开源组织，OSI）捐款，我们会感激不尽。

## Demo
### 面板
![dashboard.png](assets/images/dashboard.png)
### 上传主页
![main.png](assets/images/main.png)
### 图片库
![gallery.png](assets/images/gallery.png)
### 直链获取
![link-share.png](assets/images/link-share.png)
### 水印
![watermark.png](assets/images/watermark.png)
### 频域盲水印
![hidden_watermark.png](assets/images/hidden_watermark.png)