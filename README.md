# 精选古诗 API

一个小而克制的唐诗随机接口：50 首绝句、纯 Go 标准库、单个静态二进制，
不需要数据库、Docker 或运行时依赖。

这里的“精选”指独立转录后做过双版本核对：其中 49 首以国家图书馆藏 1933 年版
[《唐詩三百首》扫描本](https://commons.wikimedia.org/wiki/File:NLC416-17jh000616-101322_%E5%94%90%E8%A9%A9%E4%B8%89%E7%99%BE%E9%A6%96.pdf)
为主底本，并用 1705 年
[《御定全唐詩》扫描本](https://commons.wikimedia.org/wiki/Category:%E5%BE%A1%E5%AE%9A%E5%85%A8%E5%94%90%E8%A9%A9)
核对。《题都城南庄》是保留博客原有备用诗的唯一例外：指定的《唐诗三百首》
不收此诗，因此改用同为 1933 年、国家图书馆藏《本事詩》作主见证，再与
[1705 年《御定全唐詩》](https://commons.wikimedia.org/wiki/Category:%E5%BE%A1%E5%AE%9A%E5%85%A8%E5%94%90%E8%A9%A9)
核对。所用《本事詩》[扫描本在此](https://commons.wikimedia.org/wiki/File:NLC511-027032013010163-20835_%E6%9C%AC%E4%BA%8B%E8%A9%A9.pdf)。
项目不会为凑齐统一来源而伪造页码。

它是一份适合博客展示的有限选本，不宣称学术权威校勘；选目、页码、
异文判断与简繁转换见 [`data/`](data/) 和
[`docs/CURATION.md`](docs/CURATION.md)。

## 接口

```http
GET /api/poems/random
GET /api/poems/random?type=五言绝句
GET /api/poems/random?type=七言绝句
GET /healthz
```

随机接口使用 `crypto/rand`，返回 `Cache-Control: no-store`，并开放仅包含
`GET`/`OPTIONS` 的只读 CORS。非法体裁返回 `400`，不支持的业务方法返回
`405`。

```json
{
  "data": {
    "id": "tang-li-bai-jing-ye-si",
    "title": "静夜思",
    "content": [
      "床前明月光，疑是地上霜。",
      "举头望明月，低头思故乡。"
    ],
    "author": { "name": "李白" },
    "dynasty": { "name": "唐" },
    "type": { "name": "五言绝句" }
  },
  "lang": "zh-Hans"
}
```

## 本地运行

需要 Go 1.23 或更高版本：

```bash
go test ./...
go vet ./...
go run ./cmd/server
```

服务默认只监听 `127.0.0.1:8787`。如需修改，可显式设置
`POETRY_API_ADDR`；正式部署仍建议只监听回环地址，由 Nginx 反向代理。

```bash
curl http://127.0.0.1:8787/healthz
curl 'http://127.0.0.1:8787/api/poems/random?type=五言绝句'
```

## 数据门禁

服务启动前会校验嵌入数据，任一错误都会拒绝启动，包括：

- 总数恰好 50 首，五言绝句、七言绝句各 25 首；
- 每位作者最多 4 首，博客原有的 8 首备用诗全部在库；
- 每首恰好四句，每句只含五个或七个汉字；
- ID、作者与题目、正文没有重复；
- 49 首必须同时有指定 1933《唐诗三百首》和 1705《御定全唐诗》见证；
- 《题都城南庄》必须有指定 1933《本事诗》和 1705《御定全唐诗》见证；
- 异文决定、简繁转换和版本元数据结构完整。

因此未解决的缺字、作者归属或作品身份问题不能进入运行数据。

## 构建与部署

构建 Linux amd64 静态二进制：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags='-s -w -X main.version=v0.1.0' \
  -o dist/curated-poetry-api ./cmd/server
(cd dist && sha256sum curated-poetry-api > curated-poetry-api.sha256)
```

[`deploy/poetry-api.service`](deploy/poetry-api.service) 与
[`deploy/poetry-api.nginx.conf`](deploy/poetry-api.nginx.conf) 是生产模板。
推荐把版本放在 `/opt/poetry-api/releases/<version>`，再让
`/opt/poetry-api/current` 软链接指向当前版本，以便原子切换和回滚。
完整的首次安装、升级、TLS 和回滚命令见
[`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md)。

## 许可

- 程序代码：MIT，见 [`LICENSE`](LICENSE)。
- 本项目独立整理的选目、转录和结构化数据：CC0-1.0，见
  [`DATA_LICENSE`](DATA_LICENSE)。
- 来源与边界说明：[`NOTICE`](NOTICE)。
