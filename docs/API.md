# HTTP API v1

基础地址：`https://poetry-api.karenepitaya.xyz`。v1 只提供完整作品；不会返回或
暗中拼接节选，也不保证相邻请求不重复。

## 随机作品

```http
GET /api/v1/works/random
```

可选参数都是单值，多个条件取交集：

| 参数 | 合法值 | 含义 |
| --- | --- | --- |
| `collection` | `tangshi-sanbaishou-1933`、`songci-sanbaishou-zhu`、`supplemental-classics` | 选本 |
| `dynasty` | `tang`、`song` | 朝代 |
| `genre` | `shi`、`ci` | 文类 |
| `form` | `gushi`、`lushi`、`jueju`、`ci` | 体式 |
| `meter` | `5`、`7`、`mixed` | 每行字数类型 |
| `max_chars` | `1` 至 `5000` | 正文去除标点和空白后的最大字符数 |
| `script` | `hans`、`hant` | 简体或繁体；默认 `hans` |

同一个参数重复、参数值非法或出现未知参数时返回 400；合法条件没有候选作品时返回
404。候选集合中的每一首作品等概率，不按朝代或选本加权。

```json
{
  "data": {
    "id": "song-su-shi-shui-diao-ge-tou",
    "title": "水调歌头·明月几时有",
    "author": { "name": "苏轼" },
    "dynasty": { "code": "song", "name": "宋" },
    "genre": { "code": "ci", "name": "词" },
    "form": { "code": "ci", "name": "词" },
    "tune": { "name": "水调歌头" },
    "sections": [
      { "kind": "stanza", "lines": ["……", "……"] },
      { "kind": "stanza", "lines": ["……", "……"] }
    ],
    "collections": [
      { "id": "songci-sanbaishou-zhu", "position": 63 }
    ],
    "evidenceLevel": "primary-scan-reviewed"
  },
  "lang": "zh-Hans"
}
```

`tune` 只在有词牌时出现。`sections` 可以包含任意数量的序、段或阕，调用方不得假定
四句、等长诗句或恰好上下两阕。

## 健康检查

```http
GET /healthz
```

```json
{
  "status": "ok",
  "version": "v0.2.1",
  "works": 50,
  "dynasties": { "tang": 50, "song": 0 },
  "corpusRevision": "…"
}
```

## HTTP 行为

- 业务接口支持 `GET` 和预检 `OPTIONS`，开放只读 CORS。
- 随机响应和健康响应都发送 `Cache-Control: no-store`。
- 源站应用错误使用稳定结构 `{"error":{"code":"…","message":"…"}}`。
- 不支持的业务方法返回 405，并带 `Allow: GET, OPTIONS`。
- Cloudflare 对 `/api/v1/` 按来源 IP 执行边缘限流；边缘返回的 429 不属于源站
  JSON 错误契约，调用方应首先按 HTTP 状态处理并退避。

旧的 `GET /api/poems/random` 已在 `v0.2.1` 删除；该路径返回 404，客户端只应
使用版本化接口。
