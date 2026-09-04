# 诗词语料整理与校验

运行库当前包含 50 首既有唐诗和 276 首电子文本宋词。两类数据使用不同证据等级，
都必须通过结构、引用、去重和字符检查。不得把机械校验描述为扫描校勘。

## 两类来源

### 既有唐诗：primary-scan-reviewed

保留原有 50 首的正文、双扫描见证、异文和简繁转换记录。本次没有重新 OCR 或改写：

- 49 首的主见证是国家图书馆藏 1933 年春明书店本《唐诗三百首》，复核本为
  1705 年《御定全唐诗》。
- 崔护《题都城南庄》的主见证是 1933 年《本事诗》，归入 `supplemental-classics`。
- 主选本 49 首的全局目录位置仍为 `pending`，API 不返回未经核实的序号。

该等级沿用 `verified` 状态、至少两个扫描见证和完整异文检查。现有记录的审核声明
属于历史整理成果，本次扩库没有重新逐页复核。

### 新增宋词：digital-text-checked

使用 chinese-poetry/chinese-poetry 的固定提交：

`b8594f81a89752241442f2ce267d6f66f96704ee`

输入路径为 `宋词/宋词三百首.json`。原始文件与上游 MIT 许可保存在
`sources/chinese-poetry-songci/`，原文件 SHA-256 为：

`ca5d74f7fdb9d5a6acc22a7c8b4395228cad4bb8be53df44c19ba83ab6983b20`

文件实际包含 280 条，276 条导入 `songci-digital-selection`，4 条署名残缺的记录
被隔离（`韩`、`赵令`、两条 `张`）。记录索引、词牌和原因见
`import-report.json`；索引从 0 开始，可直接定位原始 JSON 数组，不猜测补齐姓名。

这不是朱孝臧扫描本的完整转录，不宣称“完整宋词三百首”。collection 保持
`in-progress`，不冒用 `songci-sanbaishou-zhu`，不填写原书位置。

- 正文、词牌和作者逐项保留源记录；原文件没有独立题名，展示题名由“词牌·首句”组成。
- 上游段落逐条保留为行，放在一个 `stanza` 容器中；不猜测上下阕，不补写题序。
- `hans` 保留上游字形（包括上游混入的繁体、异体字），`hant` 由锁定版本
  `opencc-python-reimplemented@0.1.7` 的 `s2t` 转换生成。机器转换不是原版繁体定本。
- 无名氏标为 `unknown`；其他作者沿用来源署名，`selected-edition` 不代表独立考证结论。
- 等级使用 `digital-text-checked`、状态使用 `validated`。不得填成
  `primary-scan-reviewed` / `verified`，不得伪造扫描见证。

API 返回来源提供的全部正文，不按长度截断。电子来源可能漏题序或存在文本错误；
“没有截断来源正文”不等于“已证实古籍全文完整”。

## 可重复导入

需要 uv 和 Python 3.11+；运行 API 本身仍只需要 Go，不依赖 Python。
在仓库根目录执行：

```text
uv sync --locked --project tools/songci
uv run --locked --project tools/songci tools/songci/import_songci.py
uv run --locked --project tools/songci tools/songci/import_songci.py --check
uv run --locked --project tools/songci python -m unittest discover -s tools/songci
```

`uv.lock` 固定转换依赖。导入只读取已保存的 JSON，不下载网页、扫描或调用模型。
`--check` 不写文件，重新计算所有输出并逐字节比较，包括繁体转换、导入数量和隔离报告。
有多余的生成文件时直接报错，不自动删除。更新数据源须明确更新固定版本和 SHA-256，
重新检查导入报告；修改源文本应另建可追溯的修订流程，不手改生成的作品文件。

## 验证边界

所有作品均检查 JSON 未知字段、路径、ID、作品重复、作者与题名、体裁、行 ID、
字符、选集引用和完整状态。启动时校验失败会拒绝服务。

扫描整理数据继续检查两份见证、异文重建和原有简繁规则。
电子数据检查源文件 SHA-256、记录索引，以及作者、词牌、展示题名、正文段落与固定
源记录的一致性。繁体生成结果由导入器 `--check` 验证，CI 和发布流程均执行该检查；
Go 运行时不带 OpenCC，不进行语言学意义的简繁判定。

```text
go test ./...
go vet ./...
go run ./cmd/corpuscheck
go run ./cmd/corpuscheck --files corpus/works/tang/tang-li-bai-jing-ye-si.json
```

增量校验不能代替发布前全库检查。普通缺失路径报错；只有显式
`--allow-missing --files ...` 才允许因删除/重命名退回全库检查。

扫描仅用于解决具体缺字、异文或归属疑点，不作为所有新增作品的入库前提。
结构检查、来源哈希和双文本一致性都不能证明历史文本绝对正确。

## 工作台与许可

旧文档提到的 `curated-poetry-api-workbench` 不在本次可见的常用工作目录中；本次未
重建、删除或重跑其扫描/OCR 产物。新的可复现输入和脚本已纳入当前仓库。

既有独立转录与整理记录沿用 CC0；新增上游电子数据保留 MIT 声明，不能统一改称
项目独立转录或重新去掉上游署名。详见 `NOTICE` 和保存的上游 `LICENSE`。
