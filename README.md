# 精选诗词 API

一个面向完整古典诗词的只读随机 API：纯 Go 标准库、内存目录、`crypto/rand`、
单个静态二进制，不需要数据库、Docker 或运行时依赖。

当前运行语料包含 **50 首唐诗 + 276 首宋词，共 326 首**。既有唐诗保留原扫描核对
记录；宋词从固定版本电子文本批量导入，并明确使用 `digital-text-checked` 等级。
原始 280 条宋词中，4 条署名残缺的记录被隔离，未进入随机池。

宋词分片由 [`stanzas.json`](sources/chinese-poetry-songci/stanzas.json) 记录，
参考固定版本电子文本的“○”换片标记，并单独核对漏标及异文。
分片只将既有行分组，不改动简繁正文和行 ID；单片、双片、三片、四片按作品保留。

这批宋词是电子文本选集，不宣称完整收录朱孝臧扫描本。导入只读已保存的 JSON，
不进行 OCR；来源、转换方法和异常记录均可追溯。

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

服务返回来源保存的全部正文，不按长度截断；宋词分片按固定电子参考文本核对，题序可能不完整。候选作品等概率随机，不做唐诗/宋词权重，不保证不同请求间
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

## 数据导入与整理原则

宋词筛选示例：

```text
curl 'http://127.0.0.1:8787/api/v1/works/random?collection=songci-digital-selection&script=hant'
```

- `primary-scan-reviewed`：既有 50 首唐诗，保留双扫描见证和异文校验。
- `digital-text-checked`：276 首宋词，固定来源、逐段一致性和机械检查；繁体由
  OpenCC s2t 生成，不冒充古籍扫描定本。简体字段保留上游字形，包括少量混用字形。
- 电子选集保持 `in-progress`；作品没有经过原书顺序核验，因此不返回位置。
- 扫描只用于解决具体疑点，不作为所有作品的入库前提。

原始 JSON、上游 MIT 许可证和隔离报告位于 `sources/chinese-poetry-songci/`。
导入工具使用 uv 锁定的独立 Python 环境；Go API 无新增运行依赖：

```text
uv sync --locked --project tools/songci
uv run --locked --project tools/songci tools/songci/import_songci.py
uv run --locked --project tools/songci tools/songci/import_songci.py --check
uv run --locked --project tools/songci python -m unittest discover -s tools/songci
```

`--check` 检查生成文件与固定源数据、转换结果完全一致，不写文件。具体审核等级、
文本完整性边界和重建流程见 [`docs/CURATION.md`](docs/CURATION.md)。

## 构建与部署

普通提交运行导入重现检查、测试、`go vet`、全库校验和 Linux amd64 构建。推送 `v*` 标签会创建
带 SHA-256、许可和变更说明的 GitHub Release。生产机通过明确 tag 的受控脚本
下载、校验、原子切换并在失败时回滚，不在 GitHub Actions 保存 SSH 密钥。

systemd、Nginx、Release 部署、Cloudflare 限流和 Better Stack 外部监控说明见
[`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md)。

## 许可

- 程序代码：MIT，见 [`LICENSE`](LICENSE)。
- 新增宋词电子数据保留上游 MIT 许可及署名，见 `sources/chinese-poetry-songci/LICENSE` 和 `NOTICE`。
- 本项目独立整理的转录、结构化数据与异文记录：CC0-1.0，见
  [`DATA_LICENSE`](DATA_LICENSE)。
- 古代原作、扫描来源、维基比较文本和项目边界：[`NOTICE`](NOTICE)。
