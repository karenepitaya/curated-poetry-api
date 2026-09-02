# 生产部署

生产机不需要安装 Go 或 Docker。下面的命令假定已经下载同一 GitHub Release 中的
`curated-poetry-api`、`curated-poetry-api.sha256`、`LICENSE`、`DATA_LICENSE`
和 `NOTICE`。

## 首次安装

先在下载目录核对二进制：

```bash
sha256sum -c curated-poetry-api.sha256
```

以 `v0.1.0` 为例，创建专用用户和不可变版本目录：

```bash
sudo useradd --system --home-dir /nonexistent --shell /sbin/nologin poetry-api
sudo install -d -o root -g root -m 0755 /opt/poetry-api/releases/v0.1.0
sudo install -o root -g root -m 0755 curated-poetry-api \
  /opt/poetry-api/releases/v0.1.0/curated-poetry-api
sudo install -o root -g root -m 0644 LICENSE DATA_LICENSE NOTICE \
  /opt/poetry-api/releases/v0.1.0/
sudo ln -sfn /opt/poetry-api/releases/v0.1.0 /opt/poetry-api/current.next
sudo mv -Tf /opt/poetry-api/current.next /opt/poetry-api/current
```

安装服务和 Nginx 站点：

```bash
sudo install -o root -g root -m 0644 deploy/poetry-api.service \
  /etc/systemd/system/poetry-api.service
sudo install -o root -g root -m 0644 deploy/poetry-api.nginx.conf \
  /etc/nginx/conf.d/poetry-api.conf
sudo systemctl daemon-reload
sudo systemctl enable --now poetry-api
curl -fsS http://127.0.0.1:8787/healthz
sudo nginx -t
sudo systemctl reload nginx
```

DNS 生效后签发证书；Certbot 会更新独立站点配置并由系统定时器续期：

```bash
sudo certbot --nginx -d poetry-api.karenepitaya.xyz --redirect
sudo certbot renew --dry-run
```

## 升级与回滚

升级时把新二进制放入新的版本目录，核对 SHA-256 后原子切换软链接并重启：

```bash
sudo ln -sfn /opt/poetry-api/releases/v0.2.0 /opt/poetry-api/current.next
sudo mv -Tf /opt/poetry-api/current.next /opt/poetry-api/current
sudo systemctl restart poetry-api
curl -fsS http://127.0.0.1:8787/healthz
```

回滚使用同一流程，把软链接目标换回保留的上一版本目录。切换失败时不要删除任一
版本目录；先恢复软链接，再重启并检查 `journalctl -u poetry-api`。

## 上线检查

```bash
curl -fsS https://poetry-api.karenepitaya.xyz/healthz
curl -fsS 'https://poetry-api.karenepitaya.xyz/api/poems/random?type=五言绝句'
curl -fsS 'https://poetry-api.karenepitaya.xyz/api/poems/random?type=七言绝句'
systemctl is-enabled poetry-api
systemctl is-active poetry-api
nginx -t
systemctl is-enabled certbot-renew.timer
```
