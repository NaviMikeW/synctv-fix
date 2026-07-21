# SyncTV Fix

这是基于 SyncTV 完整源码维护的群晖 Docker 定制版，包含后端、`vendors` 和 `synctv-web`，后续可以统一修改 Emby、Bilibili、播放器、聊天和移动端功能。

## 当前修复

第一阶段修复 Emby 手机端兼容问题：

- `MP4/M4V + H.264 + AAC/MP3` 可以直接播放；
- HEVC、VP9、AV1 等不确定格式由 Emby 输出 `H.264 + AAC` HLS；
- 避免手机拿到不兼容的 `original.mp4` 后显示“不能自动播放”。

## Docker 镜像

稳定镜像：

```text
ghcr.io/navimikew/synctv-fix:latest
```

每次 `main` 更新并构建成功后，`latest` 会自动更新。同时会生成不可变的提交镜像：

```text
ghcr.io/navimikew/synctv-fix:sha-<完整提交SHA>
```

## 群晖更新

使用 Compose 时：

```bash
docker compose pull
docker compose up -d
```

群晖 Container Manager 中也可以打开项目，选择“操作 → 构建/更新”重新拉取 `latest`。

示例配置见：

```text
deploy/docker-compose.synology.yml
```

首次部署前请把示例中的：

```text
/volume1/docker/synctv
```

替换成你现有 SyncTV 的真实数据目录。不要删除或覆盖现有数据目录，建议先备份。

## 安全提醒

不要把 Emby API Key、密码、Token、PlaySessionId 或完整播放 URL 提交到仓库。已经公开过的 Emby API Key 应撤销并重新生成。

## 上游与许可

本项目基于以下项目修改：

- `synctv-org/synctv`
- `synctv-org/vendors`
- `synctv-org/synctv-web`

具体固定版本见 `UPSTREAM.md`。原项目许可证和版权信息均保留。
