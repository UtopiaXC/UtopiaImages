# Utopia Images

> Yet Another Self Hoste
> d Image Hosting System By GoLang.

Utopia Images是一个自托管图床系统，集成了图片编辑、水印、多存储策略、多语言、批量导入与路径映射等特性，使用高效快速的Go语言开发。  
Demo图请见最底部。  

---

## 快速上手

### 方式一：直接运行预编译二进制文件

1. 从 [Releases 页面](https://github.com/UtopiaXC/UtopiaImages/releases) 下载适合您系统架构的压缩包。
2. 解压并在终端中启动：
   ```bash
   ./utopia_images
   ```
3. 打开浏览器访问 `http://localhost:2983`，系统将进入可视化安装向导。配置站点名称、数据库连接、邮件发信服务（SMTP）及初始化超级管理员账号后即可进入系统。

### 方式二：Docker 部署

#### 1. 使用预编译镜像运行

所有持久化数据统一归集在 `/app/data` 目录下，包含配置文件、数据库、图片存储、派生缓存与运行日志：

```bash
docker run -d \
  --name utopia_images \
  -p 2983:2983 \
  -v $(pwd)/data:/app/data \
  utopiaxc1025/utopia_images:latest
```

使用 Docker Compose：

```yaml
version: '3.8'

services:
  utopia_images:
    image: utopiaxc1025/utopia_images:latest
    container_name: utopia_images
    restart: always
    ports:
      - "2983:2983"
    volumes:
      - ./data:/app/data
    environment:
      - TZ=Asia/Shanghai
```

挂载目录说明：
- `/app/data`：包含 `config/`（配置文件 `config.yaml`）、`db/`（数据库文件）、`storage/`（原始图片存储）、`cache/`（派生图片缓存）及 `logs/`（服务运行与轮转日志）。宿主机备份此单个目录即可完整迁移整站数据与配置。

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

本人此前使用兰空图床，虽然开源版已经停止维护，但是勉强能用，所以一直没换。直到本次严重PHP漏洞，不得不将php版本更新到最新，然而兰空图床的安装和依赖极其复杂，在我更新PHP后就无法正常启动了。鉴于其复杂、反应迟钝、不再维护甚至给予PHP的现状，因此我决定彻底抛弃它转向新平台。  

因此我产生了开发一个Go版本的图床的想法。要能兼容兰空甚至其他图床的路径，要使用docker一键部署，要支持水印、防盗链、多策略等特性。
### AI使用

本项目使用Claude Opus 5.5完成需求分析与可行性验证，使用Gemini 3.8 Flash进行开发，并使用Gemini 4 Argon进行漏洞检测。  

### 开发计划与目前支持情况

由于我暂时没有对象存储的需求，因此第一版只添加了本地存储策略的支持。  

未来我计划添加对阿里云OSS、腾讯云COS、Amazon S3、GCP CGS的支持。如果有需要或者希望添加其他对象存储的支持，可以提交Issue或PR。  

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