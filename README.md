# SyncTV Fix

这是基于 SyncTV 完整源码维护的群晖 Docker 定制版，包含后端、`vendors` 和 `synctv-web`，后续可以统一修改 Emby、Bilibili、播放器、聊天和移动端功能。

## 当前修复

### 手机端播放兼容

- `MP4/M4V + H.264 + AAC/MP3` 可以直接播放；
- `MOV + H.264` 仅在音频为 AAC/MP3 时直接播放；
- WebM 视频不再被当作所有浏览器都能直接播放；
- H.264 High 10（Hi10P）不再直接播放；
- HEVC、VP9、AV1 以及其他不确定格式由 Emby 输出 `H.264 + AAC` HLS；
- 避免手机拿到不兼容的 `original.mp4` 或 WebM 后显示“不能自动播放”。

### Emby 后台播放会话

观看 Emby 影片时，SyncTV 后端会向 Emby 上报房间播放状态：

- 开始播放后，Emby 控制台显示一个 `SyncTV · 房间名` 播放会话；
- 暂停、继续、拖动和倍速变化会立即同步；
- 正常播放时每约 10 秒校准一次进度；
- 最后一个观众离开、切换影片或清空当前影片时结束会话；
- 第一版为“一个 SyncTV 房间对应一个 Emby 会话”，使用影片创建者绑定的 Emby 账号；
- 上报由 SyncTV 后端完成，浏览器不会为此获得 Emby Token。

这项功能会同步观看进度到对应的 Emby 账号。每位观众独立画质、音轨、字幕和独立 Emby 会话属于后续阶段。

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

## 验证正在运行的版本

程序版本固定为上游兼容版本 `v0.9.15`，避免 SyncTV 默认进入 `dev` 模式。定制镜像身份通过 Git commit 和 OCI 镜像标签确认：

```bash
docker exec synctv-fix synctv version
```

输出中应包含：

```text
synctv v0.9.15
- git/commit: <非空提交SHA>
```

还可以查看镜像标签：

```bash
docker inspect ghcr.io/navimikew/synctv-fix:latest \
  --format '{{ index .Config.Labels "org.opencontainers.image.version" }}'
```

标签格式为：

```text
v0.9.15-fix.<完整提交SHA>
```

没有把 `v0.9.15-fix.*` 直接作为 SyncTV 程序版本，是因为 SyncTV 自己的版本比较器会把带 `-fix` 的版本视为低于正式版 `v0.9.15` 的预发布版本，从而可能错误提示更新。

## 验证 Emby 会话上报

1. 进入 SyncTV 房间并播放一部 Emby 影片；
2. 打开 Emby 控制台，确认出现 `SyncTV · 房间名`；
3. 在 SyncTV 中暂停、继续或拖动，确认 Emby 的状态和进度随之更新；
4. 离开房间或切换影片，确认原会话消失。

如果影片由另一个 SyncTV 用户创建，会话显示在该创建者绑定的 Emby 账号下。

## 安全提醒

不要把 Emby API Key、密码、Token、PlaySessionId 或完整播放 URL 提交到仓库。已经公开过的 Emby API Key 应撤销并重新生成。

## 上游与许可

本项目基于以下项目修改：

- `synctv-org/synctv`
- `synctv-org/vendors`
- `synctv-org/synctv-web`

具体固定版本见 `UPSTREAM.md`。原项目许可证和版权信息均保留。
