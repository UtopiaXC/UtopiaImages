# UtopiaImages API 个人访问令牌 (Personal Access Token) 文档

UtopiaImages 提供了完善的 RESTful API 接口与个人访问令牌（PAT, Personal Access Token）机制，方便开发者将图床接入 PicGo、Typora、Obsidian、ShareX、CI/CD 自动化流水线或自建应用程序。

---

## 1. 令牌概述与安全说明

- **令牌格式**：以 `uimg_` 开头的加密安全随机串（如 `uimg_7f9c8d...`）。
- **认证方式**：通过标准 HTTP 请求头 `Authorization: Bearer <token>` 进行身份验证。
- **权限与生命周期**：
  - 令牌继承所属用户的所有角色权限、存储容量配额和允许使用的存储策略。
  - 可以在「个人设置 -> API 令牌」中随时新建或吊销。创建时明文仅展示一次，系统仅存储其不可逆的 SHA-256 哈希。

---

## 2. 认证请求头 (Authentication Header)

在向所有受保护的 API 接口发送请求时，必须携带以下 HTTP Header：

```http
Authorization: Bearer uimg_your_personal_access_token_here
```

若未携带或令牌无效、已过期、用户被冻结，系统将返回 `401 Unauthorized`：
```json
{
  "code": "UNAUTHORIZED",
  "message": "Authentication required"
}
```

---

## 3. 核心 API 接口定义

### 3.1 获取当前用户信息与配额 (`GET /api/v1/me`)

获取当前令牌所属用户的账号信息、存储配额及使用量。

- **URL**: `GET /api/v1/me`
- **Headers**:
  - `Authorization: Bearer uimg_...`

#### 响应示例 (`200 OK`)
```json
{
  "user": {
    "id": 1,
    "username": "admin",
    "nickname": "Administrator",
    "email": "admin@example.com",
    "status": "active",
    "role_id": 1,
    "used_bytes": 104857600,
    "role": {
      "id": 1,
      "name": "admin",
      "display_name": "超级管理员",
      "quota_bytes": 0
    }
  }
}
```
> 注：`quota_bytes = 0` 表示容量不限制。

---

### 3.2 获取可用存储策略列表 (`GET /api/v1/me/policies`)

获取当前角色被授权可用的物理存储策略。上传时可通过 `policy_id` 指定存储策略。

- **URL**: `GET /api/v1/me/policies`
- **Headers**:
  - `Authorization: Bearer uimg_...`

#### 响应示例 (`200 OK`)
```json
[
  {
    "id": 1,
    "name": "默认本地存储",
    "driver": "local",
    "url_prefix": "uploads",
    "is_default": true,
    "used_bytes": 10485760,
    "max_capacity_bytes": 0
  }
]
```

---

### 3.3 单文件/图片上传 (`POST /api/v1/upload`)

支持上传常见格式的图片（JPEG、PNG、GIF、WebP、SVG、BMP、TIFF、HEIC/HEIF 等）及静态资源。

- **URL**: `POST /api/v1/upload`
- **Headers**:
  - `Authorization: Bearer uimg_...`
  - `Content-Type: multipart/form-data`
- **Form Data 参数**：

| 字段名 | 类型 | 必填 | 说明 |
| :--- | :--- | :--- | :--- |
| `file` | Binary File | **是** | 上传的文件二进制数据 |
| `policy_id` | Integer | 否 | 指定存储策略 ID，缺省时使用用户/系统默认策略 |
| `watermark_mode` | String | 否 | 水印模式：`inherit`（跟随用户偏好）、`on`（启用）、`off`（关闭） |
| `watermark_template_id` | Integer | 否 | 指定水印模板 ID，留空使用用户默认模板 |

#### 响应示例 (`200 OK`)
```json
{
  "file": {
    "id": 42,
    "storage_policy_id": 1,
    "display_name": "example.png",
    "stored_filename": "2026/10/a1b2c3d4e5.png",
    "public_path": "uploads/2026/10/a1b2c3d4e5.png",
    "extension": "png",
    "mime_type": "image/png",
    "size": 245760,
    "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "width": 1920,
    "height": 1080,
    "is_image": true,
    "watermark_mode": "inherit",
    "created_at": "2026-10-26T21:00:00Z",
    "updated_at": "2026-10-26T21:00:00Z"
  },
  "links": {
    "url": "https://img.example.com/uploads/2026/10/a1b2c3d4e5.png",
    "markdown": "![example.png](https://img.example.com/uploads/2026/10/a1b2c3d4e5.png)",
    "html": "<img src=\"https://img.example.com/uploads/2026/10/a1b2c3d4e5.png\" alt=\"example.png\" />",
    "bbcode": "[img]https://img.example.com/uploads/2026/10/a1b2c3d4e5.png[/img]"
  }
}
```

---

### 3.4 归档压缩包导入 (`POST /api/v1/import/archive`)

支持通过归档文件批量导入图片资源，服务端自动解压、提取支持的图片并入库。

- **URL**: `POST /api/v1/import/archive`
- **Headers**:
  - `Authorization: Bearer uimg_...`
  - `Content-Type: multipart/form-data`
- **Form Data 参数**：

| 字段名 | 类型 | 说明 |
| :--- | :--- | :--- |
| `archive_file` | Binary File | 压缩包文件，支持 `.zip`、`.tar`、`.gz`、`.tar.gz` (`.tgz`)、`.7z` |
| `policy_id` | Integer | 目标存储策略 ID |
| `preserve_path` | Boolean | 是否保留压缩包内的多级目录层级结构 (`true`/`false`) |
| `strip_top_dir` | Boolean | 是否裁剪顶层同名根目录 (`true`/`false`) |
| `conflict_action` | String | 遇到同名文件处理方式：`skip`（跳过）、`overwrite`（覆盖）、`rename`（重命名） |
| `watermark_mode` | String | `inherit` / `on` / `off` |
| `hotlink_mode` | String | 防盗链模式：`inherit` / `on` / `off` |

#### 响应示例 (`200 OK`)
```json
{
  "imported": 15,
  "total": 15,
  "message": "Archive imported successfully"
}
```

---

### 3.5 查询文件列表 (`GET /api/v1/files`)

- **URL**: `GET /api/v1/files`
- **Headers**:
  - `Authorization: Bearer uimg_...`
- **Query 参数**：

| 参数 | 类型 | 说明 |
| :--- | :--- | :--- |
| `page` | Integer | 页码（默认 1） |
| `page_size` | Integer | 每页数量（默认 20，最大 100） |
| `search` | String | 文件名模糊搜索关键词 |
| `format` | String | 格式筛选：`jpeg`、`png`、`webp`、`gif`、`svg` 等 |
| `time_range` | String | 时间范围：`today`、`7d`、`30d`、`all` |
| `watermark_mode`| String | `all` / `on` / `off` / `inherit` |
| `hotlink_mode` | String | `all` / `on` / `off` / `inherit` |

#### 响应示例 (`200 OK`)
```json
{
  "items": [ /* 文件对象数组 */ ],
  "total": 128,
  "page": 1,
  "page_size": 20
}
```

---

### 3.6 单文件详情与删除

- **获取详情**: `GET /api/v1/files/:id`
- **修改属性**: `PATCH /api/v1/files/:id`
  ```json
  {
    "display_name": "new_name.png",
    "watermark_mode": "on",
    "hotlink_mode": "inherit"
  }
  ```
- **删除文件**: `DELETE /api/v1/files/:id`
- **批量删除**: `POST /api/v1/files/batch-delete`
  ```json
  {
    "ids": [12, 13, 14]
  }
  ```

---

### 3.7 生成临时免水印签名外链 (`POST /api/v1/files/:id/temp-link`)

为指定文件生成具备时效性的免水印直接访问链接。

- **URL**: `POST /api/v1/files/:id/temp-link`
- **Headers**:
  - `Authorization: Bearer uimg_...`
- **Body**:
  ```json
  {
    "expires_in": 3600
  }
  ```
- **响应示例**:
  ```json
  {
    "url": "https://img.example.com/uploads/2026/10/a1b2c3d4e5.png?token=7b92f4c9...",
    "expires_at": "2026-10-26T22:00:00Z"
  }
  ```

---

## 4. 派生图片与动态处理参数 (Image Processing Query Params)

所有通过公开路径访问的图片均支持以下实时动态处理查询参数，可直接拼接在图片 URL 后：

### 4.1 缩略图与尺寸缩放 (`size`, `w`, `h`)

- `?size=thumb`: 生成 200px 宽度比例缩略图
- `?size=small`: 生成 640px 宽度比例缩放图
- `?size=medium`: 生成 1280px 宽度比例缩放图
- `?size=large`: 生成 1920px 宽度比例缩放图
- `?w=800&h=600`: 自定义缩放到指定宽和高

### 4.2 现代格式无损/高压缩转换 (`fmt`)

- `?fmt=webp`: 动态转换为 WebP 格式（推荐，兼顾极小体积与高画质）
- `?fmt=png`: 转换为 PNG 格式
- `?fmt=jpg`: 转换为 JPEG 格式
- `?fmt=heif`: 转换为 HEIF 格式

> 示例：`https://img.example.com/uploads/photo.jpg?size=medium&fmt=webp`

---

## 5. 常见调用代码示例

### 5.1 cURL 上传图片
```bash
curl -X POST "https://img.example.com/api/v1/upload" \
  -H "Authorization: Bearer uimg_your_token_here" \
  -F "file=@/path/to/my_image.png" \
  -F "watermark_mode=inherit"
```

### 5.2 Python (requests)
```python
import requests

API_URL = "https://img.example.com/api/v1/upload"
TOKEN = "uimg_your_token_here"

headers = {
    "Authorization": f"Bearer {TOKEN}"
}

files = {
    "file": open("example.jpg", "rb")
}

data = {
    "watermark_mode": "inherit"
}

response = requests.post(API_URL, headers=headers, files=files, data=data)
res_json = response.json()

if response.status_code == 200:
    print("上传成功！外链地址:", res_json["links"]["url"])
    print("Markdown:", res_json["links"]["markdown"])
else:
    print("上传失败:", res_json)
```

### 5.3 JavaScript / Node.js (fetch)
```javascript
import fs from 'fs';
import FormData from 'form-data';
import fetch from 'node-fetch';

async function uploadImage(filePath) {
  const form = new FormData();
  form.append('file', fs.createReadStream(filePath));
  form.append('watermark_mode', 'inherit');

  const res = await fetch('https://img.example.com/api/v1/upload', {
    method: 'POST',
    headers: {
      'Authorization': 'Bearer uimg_your_token_here',
      ...form.getHeaders()
    },
    body: form
  });

  const data = await res.json();
  if (res.ok) {
    console.log('图片 URL:', data.links.url);
  } else {
    console.error('上传失败:', data);
  }
}

uploadImage('./screenshot.png');
```

### 5.4 PicGo 自定义 Web 图床配置
在 PicGo 客户端中，选择 **图床设置 -> 自定义 Web 图床**：
- **API 地址**: `https://img.example.com/api/v1/upload`
- **POST 参数名**: `file`
- **JSON 路径**: `links.url`
- **自定义请求头**: `{"Authorization": "Bearer uimg_your_token_here"}`
- **自定义 Body**: `{"watermark_mode": "inherit"}`
