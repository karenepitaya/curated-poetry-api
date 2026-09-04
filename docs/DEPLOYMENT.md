# 生产部署

生产机不安装 Go 或 Docker。GitHub Release 提供静态 Linux amd64 二进制、
SHA-256、代码许可、数据许可和来源声明；生产机只负责校验并切换版本。

## 首次安装

创建无登录用户和版本目录：

```bash
sudo useradd --system --home-dir /nonexistent --shell /sbin/nologin poetry-api
sudo install -d -o root -g root -m 0755 /opt/poetry-api/releases
```

安装仓库中的 systemd、Nginx 和受控部署脚本：

```bash
sudo install -o root -g root -m 0644 deploy/poetry-api.service \
  /etc/systemd/system/poetry-api.service
sudo install -o root -g root -m 0644 deploy/poetry-api.nginx.conf \
  /etc/nginx/conf.d/poetry-api.conf
sudo install -o root -g root -m 0755 deploy/deploy-release.sh \
  /usr/local/sbin/deploy-poetry-api
sudo systemctl daemon-reload
sudo nginx -t
sudo systemctl reload nginx
```

DNS 生效后由 Certbot 签发并自动续期 HTTPS 证书：

```bash
sudo certbot --nginx -d poetry-api.karenepitaya.xyz --redirect
sudo certbot renew --dry-run
```

## 发布与升级

推送 `v*` 标签会触发 Release workflow。它在创建 Release 前执行测试、`go vet`、
全库校验和 Linux amd64 构建，并随二进制发布校验文件与许可。GitHub Actions
不保存生产机 SSH 密钥。

Release 完成后，在生产机手动指定明确版本：

```bash
sudo /usr/local/sbin/deploy-poetry-api v0.2.1
```

脚本依次下载同一 Release 的文件、核对 SHA-256、创建
`/opt/poetry-api/releases/<tag>`、原子切换 `/opt/poetry-api/current`、重启服务，
再检查回环健康接口、公网健康接口和新随机接口。任何部署后检查失败都会切回原来的
软链接并重启；失败的暂存目录或新版本目录会被精确移除。已存在的 tag 目录不会被
覆盖，同一 tag 需要恢复时应直接回滚，而不是重新部署。成功后只保留当前版本和前
两个版本。下载和探活都有硬超时；Release 下载会保留已收到的部分并在有限次数内
续传。服务重启和回滚后会在有界时间内等待健康接口就绪，网络半开或进程尚未开始
监听都不会让部署无限挂起，也不会因第一次连接被拒就过早判定失败。

如需手工回滚，仍使用同样的原子切换方式：

```bash
sudo ln -sfn /opt/poetry-api/releases/v0.1.0 /opt/poetry-api/.current.next
sudo mv -Tf /opt/poetry-api/.current.next /opt/poetry-api/current
sudo systemctl restart poetry-api
curl -fsS http://127.0.0.1:8787/healthz
```

## 上线检查

```bash
curl -fsS http://127.0.0.1:8787/healthz
curl -fsS https://poetry-api.karenepitaya.xyz/healthz
curl -fsS 'https://poetry-api.karenepitaya.xyz/api/v1/works/random?max_chars=120&script=hans'
curl -fsS 'https://poetry-api.karenepitaya.xyz/api/v1/works/random?collection=supplemental-classics&script=hant'
systemctl is-enabled poetry-api
systemctl is-active poetry-api
systemctl show poetry-api -p MemoryCurrent -p MemoryPeak -p NRestarts
nginx -t
systemctl is-enabled certbot-renew.timer
```

发布前的回环负载门禁为 1,000 请求、并发 20、零错误、p95 小于 50 ms；公网
30 请求要求零错误，p95 小于 1.5 s。公网数字是部署观测门槛，不通过缓存随机响应
来掩盖链路延迟。完整语料版本的进程峰值必须低于 48 MiB，systemd 的
`MemoryMax=64M` 不自动上调。

## Cloudflare 限流

为主机 `poetry-api.karenepitaya.xyz` 创建一条 Rate limiting rule：匹配
`/api/v1/`，按来源 IP 在 10 秒内最多 100 次，超过后阻断 10 秒并返回 429。
`/healthz` 不在匹配路径中。随机响应继续使用 `Cache-Control: no-store`，不配置
共享缓存。

边缘生成的 429 由 Cloudflare 返回，不应被误解为 Go 服务的 JSON 业务错误；
调用方必须按 HTTP 状态处理它。

## 外部监控

Better Stack 从服务外部约每 3 分钟检查
`https://poetry-api.karenepitaya.xyz/healthz`，同时启用故障和恢复邮件。监控不得
部署在同一台生产机上，否则机器或网络整体失联时也无法告警。
