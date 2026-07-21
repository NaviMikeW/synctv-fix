# SyncTV Mobile Fix

这是给群晖 Docker 使用的 SyncTV 定制构建。第一阶段只处理已经确认的手机播放兼容问题。

> 当前正在迁移为完整源码仓库：SyncTV 后端、vendors 和 synctv-web 会全部展开到同一个仓库中，便于后续统一修改。导入完成后会改用新的完整源码 PR。

## 已确认的问题

SyncTV 为整个房间只生成一个 Emby 播放地址，并向 Emby 声明 MP4 容器中的 HEVC、VP9、AV1 都可以直接播放。因此部分 `MP4 + HEVC` 文件会被返回为：

```text
/Videos/{id}/original.mp4
```

在 iOS Safari 上，这类原始文件可能无法播放；而触发 Emby HLS 转码的 MKV/ASS 文件反而可以播放。

## 第一阶段修复

本分支采用保守的房间级兼容策略：

- `MP4/M4V + H.264 + AAC/MP3` 继续直接播放；
- MP4 中的 HEVC、VP9、AV1 不再直接播放；
- 其他不兼容视频由 Emby 转为 `H.264 + AAC` HLS；
- 不修改媒体文件；
- 不修改 SyncTV 数据库、用户、房间和聊天功能；
- Bilibili 等其他 vendor 保持原状。

这是房间级兼容修复，不是第二阶段的“每个用户独立清晰度、音轨和字幕”。

## 安全提醒

不要把 Emby API Key、密码、Token、PlaySessionId 或完整播放 URL 提交到仓库。已经公开过的 Emby API Key 应立即撤销并重新生成。

## 自动构建镜像

GitHub Actions 会构建 AMD64 镜像，适用于群晖 DS1821+。

## 群晖测试原则

1. 先备份现有 SyncTV 数据目录。
2. 记录原容器使用的端口、数据卷和环境变量。
3. 暂停原容器，不要删除。
4. 新容器必须映射到同一个 `/root/.synctv` 数据目录。
5. 首次测试建议临时改用其他端口，例如 `8081:8080`，验证后再替换正式容器。

## 上游与许可

本项目基于 [synctv-org/synctv](https://github.com/synctv-org/synctv)、[synctv-org/vendors](https://github.com/synctv-org/vendors) 和 [synctv-org/synctv-web](https://github.com/synctv-org/synctv-web) 修改。原项目许可证会完整保留。
