# 精选诗词 API

一个面向完整古典诗词的只读随机 API：纯 Go 标准库、内存目录、`crypto/rand`、
单个静态二进制，不需要数据库、Docker 或运行时依赖。

项目把“运行时可用”和“整本收录完成”分开。`v0.2.x` 先完成通用数据模型和 API
迁移，运行池仍是已经核对的 50 首唐代绝句；只有指定的 1933 年版
《唐诗三百首》目录全部通过门禁后才发布 `v0.3.0`，朱孝臧本《宋词三百首》同理
在 `v0.4.0` 才宣称完整。未解决的缺字、归属或作品身份问题只留在工作区，不进入
线上随机池。

## API

```http
GET /api/v1/works/random
GET /healthz
```

随机接口支持 `collection`、`dynasty`、`genre`、`form`、`meter`、`max_chars`
和 `script` 单值参数，多个条件取交集。例如：

```bash
curl 'http://127.0.0.1:8787/api/v1/works/random?max_chars=120&script=hans'
curl 'http://127.0.0.1:8787/api/v1/works/random?genre=shi&form=jueju&meter=5&script=hant'
```

服务始终返回完整作品。候选作品等概率随机，不做唐诗/宋词权重，不保证不同请求间
不重复。随机响应使用 `Cache-Control: no-store`，并开放仅含 `GET`/`OPTIONS` 的
只读 CORS。参数重复或非法返回 400，无匹配作品返回 404。

完整参数、响应和错误契约见 [`docs/API.md`](docs/API.md)。旧的
`/api/poems/random` 已在 `v0.2.1` 删除。

## 本地开发

需要 Go 1.23 或更高版本：

```bash
go test ./...
go vet ./...
go run ./cmd/corpuscheck
go run ./cmd/server
```

服务默认只监听 `127.0.0.1:8787`。可用 `POETRY_API_ADDR` 显式修改；生产环境仍
只监听回环地址，由 Nginx 反向代理。

只改少量作品时可先做增量检查：

```bash
go run ./cmd/corpuscheck --files \
  corpus/works/tang/tang-li-bai-jing-ye-si.json \
  corpus/collections/tangshi-sanbaishou-1933.json
```

增量检查缩短本地反馈，不替代 CI、标签发布前的全库一致性检查。
如果变更列表包含已删除或重命名的作品文件，须显式允许缺失路径；命令会改做并明确
报告全库检查，普通拼写错误仍会失败：

```bash
go run ./cmd/corpuscheck --allow-missing --files \
  corpus/works/tang/<deleted-work-id>.json
```

## 数据结构

```text
corpus/
├── works/<dynasty>/<work-id>.json
├── editions/<edition-id>.json
├── collections/<collection-id>.json
└── normalization.json
```

每个作品文件同时保存正文、稳定行 ID、体裁、可选词牌、作者归属状态、审核状态、
扫描定位、异文和语境相关的简繁覆盖。通用机械简繁规则集中在
`normalization.json`。collection manifest 保存所选版本的目录、原书顺序和整理
进度；只有每个成员均通过校验后才允许标记 `complete`。

程序用 `go:embed` 嵌入整个 `corpus`，按路径稳定加载。启动时全库校验失败会拒绝
提供服务。校验可以证明结构、引用和记录彼此一致，但不能替代对扫描文字的人工
判读。

## 整理原则

- 唐诗主本：国家图书馆藏 1933 年版《唐诗三百首》；固定版本的维基文库文本只做
  机器差异检测，疑义再查《御定全唐诗》等第二扫描。
- 宋词主本：朱孝臧《宋词三百首》上下册扫描；固定版本的维基文库文本用于交叉
  检查，疑义再查《宋词三百首笺》。
- “完整”是所选版本目录中的全部作品和原书顺序，不强行解释为恰好 300 首。
- 《题都城南庄》等不在主本目录的现有作品归入 `supplemental-classics`，不冒充
  选本成员。
- 繁体主文本与简体展示文本并存；不确定或有争议的作者归属保留选本署名，并显式
  标记状态。
- 机器文本只负责定位和报警；每首进入运行池前仍须目视核对主扫描。

具体来源、页码口径、审核等级和维护流程见
[`docs/CURATION.md`](docs/CURATION.md) 与 [`NOTICE`](NOTICE)。

## 构建与部署

普通提交运行测试、`go vet`、全库校验和 Linux amd64 构建。推送 `v*` 标签会创建
带 SHA-256、许可和变更说明的 GitHub Release。生产机通过明确 tag 的受控脚本
下载、校验、原子切换并在失败时回滚，不在 GitHub Actions 保存 SSH 密钥。

systemd、Nginx、Release 部署、Cloudflare 限流和 Better Stack 外部监控说明见
[`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md)。

## 许可

- 程序代码：MIT，见 [`LICENSE`](LICENSE)。
- 本项目独立整理的转录、结构化数据与异文记录：CC0-1.0，见
  [`DATA_LICENSE`](DATA_LICENSE)。
- 古代原作、扫描来源、维基比较文本和项目边界：[`NOTICE`](NOTICE)。
