# Baby Tracker for fnOS

[![Download Latest FPK](https://img.shields.io/badge/Download-Latest%20FPK-0ea5e9?style=for-the-badge&logo=github)](https://github.com/FNOSP/Baby_tracker-fnos/releases/latest)
[![fnOS](https://img.shields.io/badge/fnOS-native%20FPK-0f766e?style=flat-square)](#)
[![Platform](https://img.shields.io/badge/platform-x86__64%20%7C%20arm64-334155?style=flat-square)](#)
[![Go](https://img.shields.io/badge/Go-native-00ADD8?style=flat-square&logo=go&logoColor=white)](#)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue?style=flat-square)](LICENSE)

Baby Tracker 的独立飞牛 fnOS FPK 仓库
应用后端采用 Go 原生二进制运行，支持 x86_64 与 arm64，接入飞牛统一网关和账号体系，数据保存在本地。

## 下载

[**下载最新 FPK**](https://github.com/FNOSP/Baby_tracker-fnos/releases/latest) · [查看全部 Releases](https://github.com/FNOSP/Baby_tracker-fnos/releases)

每个 Release 包含：

- `baby-tracker.fpk`：飞牛安装包
- `baby-tracker.fpk.sha256`：SHA256 校验文件
- `SOURCE_INFO.txt`：对应源码提交信息

## 功能简介

覆盖日常照护、成长趋势、疫苗规划和家庭自动化，数据全部保存在飞牛本地。

| 功能 | 说明 |
| --- | --- |
| 总览 | 今日奶量目标、喂养次数、尿便次数、上次喂养和快捷记录。 |
| 趋势 | 按日、月或时段查看体重、每日喂养量和目标达成趋势。 |
| 成长 | 疫苗规划、健康随访、自定义日程、倒计时和接种进度。 |
| 家庭共享 | 使用飞牛账号登录，多设备访问同一份本地数据。 |
| Home Assistant | 提供传感器状态、操作按钮和独立 API Key 鉴权。 |
| MCP | 通过 Streamable HTTP 为支持 MCP 的客户端提供读取与记录工具。 |

<table>
  <tr>
    <td align="center" width="33%"><a href="docs/screenshots/dashboard.jpg"><img src="docs/screenshots/dashboard.jpg" width="100%" alt="总览"></a><br><sub>总览</sub></td>
    <td align="center" width="33%"><a href="docs/screenshots/trends.jpg"><img src="docs/screenshots/trends.jpg" width="100%" alt="趋势"></a><br><sub>趋势</sub></td>
    <td align="center" width="33%"><a href="docs/screenshots/growth.jpg"><img src="docs/screenshots/growth.jpg" width="100%" alt="成长"></a><br><sub>成长</sub></td>
  </tr>
</table>

近期用户可见更新见 [CHANGELOG.md](CHANGELOG.md)。应用中心介绍保持稳定，功能与兼容性变化统一记录在 Changelog。

## 飞牛适配

- 原生 fnOS 应用，不使用 Docker，支持 x86_64 和 arm64。
- 接入飞牛统一网关、桌面入口和账号体系。
- 首次登录强制设置管理密码，不预置默认管理员。
- SQLite 数据保存在 `data-share`，升级或卸载不会误删数据。
- Go 运行时无需额外语言环境，前端依赖内置，不依赖公网 CDN。
- 默认仅通过飞牛统一网关访问，可按需开放 TCP 监听。

## 配置指南

| 配置项 | 操作 |
| --- | --- |
| 首次登录 | 从飞牛桌面打开应用，使用当前飞牛账号登录并设置管理密码。 |
| 网络访问 | 默认只通过飞牛统一网关访问；在“管理 > 管理配置”同时填写监听地址和端口后，才开放 TCP 访问。 |
| Home Assistant | 在“管理 > 管理配置”生成 API Key，再按页面生成的 YAML 配置服务地址。 |
| MCP | 先开放额外 TCP 监听，再在“管理配置”打开 MCP 开关并生成独立 API Key。统一网关不提供 MCP。 |
| 数据备份 | 数据默认位于 `baby-tracker/data`；建议导出 JSON，直接备份 SQLite 前先停止应用并一并复制 WAL 文件。 |

## MCP 接入

MCP 默认关闭。先在“管理 > 管理配置”同时设置监听地址与端口，再打开 MCP 开关；开关打开后会在原位置展开端点和密钥配置。只有同时满足显式开启与直连监听有效时，应用才会在额外 TCP 监听上开放：

```text
http://<监听地址或 NAS 地址>:<端口>/mcp
```

请求使用 Streamable HTTP，并通过 Bearer 密钥鉴权：

```http
Authorization: Bearer <MCP_API_KEY>
```

管理页可生成或轮换独立 MCP API Key，旧密钥会在轮换后立即失效。关闭 MCP 开关会立即停止 `/mcp` 端点，即使监听端口仍然开放。当前提供每日概览、记录列表、宝宝资料、快捷按钮、创建记录和使用快捷按钮等工具。

MCP 可访问的数据、工具权限、网络边界和未开放功能见 [MCP 功能与权限说明](docs/MCP_PERMISSIONS.md)。

安全边界：

- 通过飞牛统一网关访问的 `/app/baby-tracker/mcp` 始终返回 `Not Found`。
- Unix Socket 和带网关身份头的请求不会开放 MCP。
- MCP 仅接受额外的直连 TCP 监听，请优先限制在局域网或 VPN 内使用。
- 服务会校验 Origin、限制请求体大小，并按客户端执行每分钟 60 次限流。

## 安装与使用

系统要求：fnOS `1.1.3100` 或更高版本。

1. 从 [最新 Release](https://github.com/FNOSP/Baby_tracker-fnos/releases/latest) 下载 `baby-tracker.fpk`。
2. 在飞牛应用中心选择“手动安装”，或执行：

```bash
appcenter-cli install-fpk ./baby-tracker.fpk
```

3. 从飞牛桌面打开应用，使用当前飞牛账号登录，并设置管理密码。

数据目录通常对应 `/var/apps/baby-tracker/share/data`。

安装包只包含 Go 原生二进制和静态前端资源，不会声明或安装 Python 依赖。

## 构建与发布

构建要求：Go（版本见 `go.mod`）、`bash`、`git`、`curl`、`jq`、`perl`。

```bash
git clone https://github.com/FNOSP/Baby_tracker-fnos.git
cd Baby_tracker-fnos
./scripts/test.sh
./scripts/build.sh
```

安装包生成到 `dist/baby-tracker.fpk`，其中同时包含 linux/amd64 与 linux/arm64 Go 二进制，启动时由 fnOS 启动脚本按 CPU 架构选择。

推送 `v*` 标签后，[release.yml](.github/workflows/release.yml) 会自动构建、测试并发布 FPK：

```bash
git tag v1.8.1
git push origin v1.8.1
```

标签版本必须与 `packaging/baby-tracker/manifest` 中的 `version` 一致。

## 目录

```text
cmd/        Go 服务入口
internal/   Go Web、鉴权、数据库和业务逻辑
docs/        README 截图等文档资源
source/      页面模板与前端静态资源
packaging/   fnOS 应用包、Manifest、向导和运行资源
scripts/     构建、代码适配和测试脚本
dist/        构建产物
```

## 许可证与上游

- 许可证：[GNU GPL v3](LICENSE)
- 原 Docker 版：[XiGeMaX/Baby_tracker](https://github.com/XiGeMaX/Baby_tracker)
- 本项目为第三方飞牛 fnOS 适配工程，不是飞牛官方项目。
