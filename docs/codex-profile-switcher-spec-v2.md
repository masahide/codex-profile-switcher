# Codex Profile Switcher `cx` 実装仕様書 改訂版

更新日: 2026-09-06  
仕様版: v2

## 1. 目的

OpenAI Codex CLI の実行環境をプロファイルごとに分離し、ChatGPT 認証と OpenAI API キー認証を簡単かつ安全に切り替えて利用できるクロスプラットフォーム CLI を Go で実装する。

ツール名は `cx` とする。

主な用途は以下。

- ChatGPT account authentication の Codex 利用枠を使う（Free、Go、Plus、Pro など）
- OpenAI API キー認証へ切り替えて API 側の利用枠を使う
- Data Sharing の complimentary daily tokens を利用する
- complimentary daily tokens の当日利用状況を CLI から確認しやすくする
- Windows、WSL、macOS で同じ操作体系を使う
- 将来的に `local`、`work` など追加プロファイルへ拡張可能にする

認証方式として区別するのは ChatGPT プラン（Free、Go、Plus、Pro など）ではなく、ChatGPT account authentication と OpenAI API key authentication である。ChatGPT プランを変更しても、ChatGPT account authentication の profile 名は変わらない。

---

# 2. この改訂版での重要な整理

## 2.1 OpenAI が公式に案内している無料トークン確認方法

OpenAI Help Center が公式に案内している complimentary daily tokens の確認方法は OpenAI Platform の Usage Dashboard である。

公式には以下の 2 通りが案内されている。

### 方法 A

Usage と Costs を比較する。

- Usage には complimentary tokens を含む
- complimentary tokens は Costs には課金として現れない
- input tokens と output tokens の両方を表示する

### 方法 B

Chat Completions Usage Dashboard で `service tier` によってグループ化する。

無料分は Dashboard 上で以下として表示される。

```text
data sharing incentive tier - input tokens
data sharing incentive tier - output tokens
```

両者を合計したものが complimentary token として処理された token 使用量である。

Dashboard:

```text
https://platform.openai.com/usage/chat-completions
```

OpenAI Help:

```text
https://help.openai.com/en/articles/10306912
```

## 2.2 `cx quota` は公式残量 API のラッパーではない

現時点の公開 OpenAI API には、

```text
complimentary_tokens_remaining
```

のような authoritative な無料枠残量を直接返す API は確認できない。

そのため `cx quota` は、

- OpenAI Organization Usage API
- OpenAI Help Center に公開されている quota policy
- 利用者が指定する Usage Tier
- Data Sharing 対象 project の scope

から利用状況を再構成し、推定値を表示する補助機能とする。

したがって以下のような名称を使う。

```text
estimated remaining
estimated complimentary usage
```

以下のように authoritative な値であるかのようには表示しない。

```text
official remaining
exact remaining
billing balance
```

## 2.3 Dashboard を最終的な確認元とする

`cx quota` の計算結果と Dashboard が異なる場合は Dashboard を優先する。

README と `cx quota --help` にこの制約を明記する。

特に以下は API から完全には判定できない可能性がある。

- Organization が complimentary token offer の対象であるか
- Data Sharing が現在有効であるか
- Data Sharing が全 project か selected projects か
- Dashboard 上の `data sharing incentive tier` が Usage API の raw `service_tier` にどの文字列で返るか
- policy change が発生した直後
- Usage API や Dashboard の集計遅延

---

# 3. 最重要設計方針

## 3.1 Codex の認証ファイルを直接切り替えない

`cx` は以下を行ってはならない。

- Codex の `auth.json` をコピーする
- Codex の `auth.json` を編集する
- Codex の `auth.json` を解析する
- ChatGPT OAuth token を直接扱う
- Codex 用 API key を直接保存する

代わりに、プロファイルごとに異なる `CODEX_HOME` を設定して Codex CLI を子プロセスとして起動する。

```text
<UserHome>/.codex-profiles/
├── chatgpt/
│   ├── auth.json
│   └── config.toml
└── api/
    ├── auth.json
    └── config.toml
```

credential storage と token refresh は Codex 本体へ委譲する。

`CODEX_HOME` は認証だけでなく Codex の設定・状態全体の root である。そのため `config.toml`、keyring 用の credential key、その他のローカル状態も profile ごとに独立し、既存の `<UserHome>/.codex/config.toml` は自動的には読み込まれない。設定の自動同期は MVP の対象外とする。

## 3.2 `cx` は Codex ランチャーとして振る舞う

```bash
cx chatgpt
```

は概念的に以下と同等。

```bash
CODEX_HOME="$HOME/.codex-profiles/chatgpt" codex
```

```bash
cx api
```

は概念的に以下と同等。

```bash
CODEX_HOME="$HOME/.codex-profiles/api" codex
```

Go では `CODEX_HOME` を child process の環境変数としてのみ設定する。

```go
cmd := exec.Command(codexBin, args...)
cmd.Env = append(os.Environ(), "CODEX_HOME="+profileDir)
```

親 process や親 shell の `CODEX_HOME` は変更しない。

## 3.3 認証処理は Codex 本体へ委譲する

ChatGPT account authentication。

```text
CODEX_HOME=<chatgpt-dir> codex login
```

API key login。

```text
printenv OPENAI_API_KEY | CODEX_HOME=<api-dir> codex login --with-api-key
```

PowerShell では以下の形式とする。

```powershell
$env:CODEX_HOME = '<api-dir>'
$env:OPENAI_API_KEY | codex login --with-api-key
```

`--with-api-key` は stdin から key を読むため、TTY から直接実行した場合は次の案内を表示して exit code 2 とする。

```text
cx: API login requires the API key on stdin

  printenv OPENAI_API_KEY | cx login api
```

`cx` 自身は Codex credential の内部形式に依存しない。

## 3.4 Usage API 用 credential は Codex credential と分離する

OpenAI Organization Usage API は Administration API の一部であり、公式 API Reference の例でも Admin API key が使用される。

`cx quota` 用 credential と Codex がモデル実行に使う Project API key は別物として扱う。

MVP では Admin API key を保存しない。

以下の環境変数からのみ読む。

```text
OPENAI_ADMIN_KEY
```

`cx` はその値を以下へ出してはならない。

- stdout
- stderr
- debug log
- exception
- telemetry
- command line argument

---

# 4. 対応プラットフォーム

最低限以下を対象とする。

- Windows amd64
- Windows arm64
- Linux amd64
- Linux arm64
- macOS amd64
- macOS arm64

WSL は Linux として扱う。

Go のクロスコンパイルで release binary を生成できるようにする。

---

# 5. 技術方針

MVP は Go 標準ライブラリのみで実装する。

CLI framework は使用しない。

利用候補。

```text
os
os/exec
path/filepath
fmt
errors
strings
net/http
net/url
encoding/json
time
context
strconv
```

MVP では以下を導入しない。

```text
Cobra
Viper
OpenAI SDK
```

理由。

- CLI が小さい
- Usage API は単純な HTTP GET で利用できる
- binary を軽量に保つ
- dependency surface を小さくする
- API response と quota 計算を自前の小さい型で十分表現できる

---

# 6. 非目標

MVP では以下を実装しない。

- Codex API key の保存
- Admin API key の保存
- credential の独自暗号化
- OS Keychain の直接操作
- Windows Credential Manager の直接操作
- Codex `auth.json` の読み書き
- Codex `auth.json` の構造解析
- ChatGPT OAuth token の取得や更新
- Codex の自動インストール
- GUI
- OpenAI Platform の Data Sharing 設定変更
- complimentary token offer への自動 opt-in
- OpenAI billing 設定変更
- Usage Tier の自動変更
- OpenAI Dashboard の private API 利用
- Dashboard の HTML scraping
- browser automation
- Dashboard 表示値を private endpoint から取得する実装
- undocumented API を利用した残量取得

公開 API と公開仕様のみを使用する。

---

# 7. CLI 名

実行ファイル。

```text
cx
```

リポジトリ名候補。

```text
codex-profile-switcher
```

Go module は実際の repository path に合わせる。

---

# 8. デフォルトプロファイル

MVP では以下の 2 profile を組み込みで提供する。

| profile | purpose |
| --- | --- |
| `chatgpt` | ChatGPT account authentication |
| `api` | OpenAI API key authentication |

将来的に以下へ拡張できる内部設計にする。

```text
local
work
test
```

MVP では任意 profile の追加や削除コマンドは不要。

---

# 9. プロファイル保存場所

デフォルト root。

```text
<UserHome>/.codex-profiles
```

Go では `os.UserHomeDir()` を利用する。

各 profile の `CODEX_HOME`。

```text
<UserHome>/.codex-profiles/<profile>
```

Linux、WSL、macOS。

```text
/home/user/.codex-profiles/chatgpt
/home/user/.codex-profiles/api
```

Windows。

```text
C:\Users\user\.codex-profiles\chatgpt
C:\Users\user\.codex-profiles\api
```

## 9.1 `CX_HOME`

profile root を以下で上書き可能とする。

```text
CX_HOME
```

例。

```bash
CX_HOME=/mnt/d/codex-profiles cx api
```

PowerShell。

```powershell
$env:CX_HOME = "D:\codex-profiles"
cx api
```

## 9.2 `CX_CODEX_BIN`

Codex executable は通常以下で探索する。

```go
exec.LookPath("codex")
```

次の環境変数が指定されていれば優先する。

```text
CX_CODEX_BIN
```

---

# 10. CLI 仕様

## 10.1 Codex 起動

```text
cx chatgpt [codex args...]
cx api [codex args...]
```

例。

```bash
cx chatgpt
cx api
cx api exec "このPRをレビューして"
cx api --model gpt-5.6-sol
```

Codex arguments は原則として順序を変えずそのまま透過する。

`--` の挿入を必須にしない。

## 10.2 login

```text
cx login chatgpt
printenv OPENAI_API_KEY | cx login api
```

### chatgpt

内部実行。

```text
CODEX_HOME=<chatgpt-dir> codex login
```

### api

内部実行。

```text
printenv OPENAI_API_KEY | CODEX_HOME=<api-dir> codex login --with-api-key
```

stdin、stdout、stderr は Codex process へ接続する。

以下のような secret を command argument で受け取る機能は禁止。

```text
cx login api --key sk-...
```

## 10.3 status

```bash
cx status
cx status chatgpt
cx status api
```

各 profile について以下を実行する。

```text
CODEX_HOME=<profile-dir> codex login status
```

`auth.json` は解析しない。

全 profile の確認では 1 profile の失敗で残りを中断しない。

## 10.4 path

```bash
cx path chatgpt
cx path api
```

stdout には path のみ表示する。

## 10.5 profiles

```bash
cx profiles
```

MVP 出力。

```text
chatgpt
api
```

## 10.6 help

```text
cx
cx help
cx --help
cx -h
```

引数なしは help を表示し exit code 0。

---

# 11. complimentary token 確認コマンド

## 11.1 command

```bash
cx quota
```

詳細表示。

```bash
cx quota --verbose
```

JSON。

```bash
cx quota --json
```

Usage Tier 指定。

```bash
cx quota --usage-tier 2
cx quota --usage-tier 4
```

Data Sharing 対象 project を限定。

```bash
cx quota --project proj_xxx
```

複数指定。

```bash
cx quota --project proj_xxx --project proj_yyy
```

## 11.2 command の意味

`cx quota` は次を目的とする。

1. 当日 00:00 UTC 以降の OpenAI API token usage を取得する
2. complimentary token 対象 model group ごとに使用量を集計する
3. Usage Tier（未指定時は 1）に応じて公開 quota policy から推定残量を計算する
4. API で確認できる `service_tier` を diagnostics として表示する
5. Dashboard で確認すべき公式な確認方法を案内できるようにする

`cx quota` は無料トークンの authoritative balance API ではない。

---

# 12. 公式確認と API 推定を区別する

## 12.1 Dashboard の情報

以下は OpenAI Help Center に明記されている。

- Usage Dashboard の usage activity には free tokens が含まれる
- Costs には free tokens が cost として現れない
- Chat Completions Dashboard を `service tier` で group すると無料分を確認できる
- Dashboard 上では `data sharing incentive tier - input/output tokens` と表示される

これは `cx` の仕様上 `official verification method` と扱う。

## 12.2 Organization Usage API の情報

公開 Organization Usage API は以下を提供する。

```text
GET /v1/organization/usage/completions
```

少なくとも以下を取得できる。

```text
input_tokens
output_tokens
input_cached_tokens
model
project_id
api_key_id
batch
service_tier
num_model_requests
```

また `group_by` は以下を含む複数フィールドをサポートする。

```text
project_id
user_id
api_key_id
model
batch
service_tier
```

API response には pagination として以下がある。

```text
has_more
next_page
```

## 12.3 API に関して仮定してはいけないこと

OpenAI Help Center は Dashboard 上の表示名を説明しているが、

```text
data sharing incentive tier
```

が Organization Usage API の raw `service_tier` に何という literal value で返るかまでは公開仕様として明記していない。

したがって MVP では以下を禁止する。

```go
if result.ServiceTier == "data_sharing_incentive" {
    // official free usage
}
```

のような未文書化 literal の決め打ち。

同様に以下も禁止する。

- Dashboard private API を reverse engineer する
- browser network request をコピーして利用する
- HTML を scrape して数値を抽出する
- service tier 名を根拠なく推測する

---

# 13. `cx quota` の計算方式

## 13.1 基本方針

MVP の残量推定は、

```text
公開 quota
-
当日 Data Sharing 対象 scope の eligible model token traffic
```

を基本とする。

この値を以下として表示する。

```text
estimated remaining
```

無料として実際に処理された token 数そのものだとは断定しない。

## 13.2 集計期間

complimentary token counter は OpenAI Help Center の記載に従い毎日、

```text
00:00 UTC
```

に reset される。

そのため、

```go
now := time.Now().UTC()

start := time.Date(
    now.Year(),
    now.Month(),
    now.Day(),
    0, 0, 0, 0,
    time.UTC,
)
```

を利用する。

API request。

```text
start_time=<UTC day start>
end_time=<now>
bucket_width=1d
group_by=model
group_by=service_tier
```

project scope を指定する場合は `project_ids` も渡す。

## 13.3 token 数

基本 token 数。

```text
input_tokens + output_tokens
```

`input_cached_tokens` は `input_tokens` の breakdown であり、別途加算して二重計上しない。

fine-tuned model は対象外なので eligible model list に一致しないものは quota pool に含めない。

tool-specific cost は completions token quota と混同しない。

## 13.4 quota crossing

OpenAI Help Center では、新しい request が残り quota を超える場合、その request 全体が通常課金になると説明されている。

例。

```text
current total: 975,000
next request:   30,000
quota:       1,000,000
```

この request は一部 25,000 token だけ無料になるのではなく、30,000 token 全体が billed になる。

このため `estimated remaining` は、

```text
次にその token 数まで無料で使える保証
```

を意味しない。

表示時にその旨を注記する。

---

# 14. Usage API

## 14.1 endpoint

```text
GET https://api.openai.com/v1/organization/usage/completions
```

## 14.2 authentication

```text
Authorization: Bearer $OPENAI_ADMIN_KEY
```

OpenAI API Reference の Organization Usage のコード例では Admin API key が利用される。

## 14.3 request parameters

最低限。

```text
start_time
end_time
bucket_width=1d
group_by=model
group_by=service_tier
```

必要に応じて。

```text
project_ids
page
limit
```

pagination の `has_more` と `next_page` を処理する。

## 14.4 timeout

HTTP timeout。

```text
10 seconds
```

程度をデフォルトとする。

将来必要なら設定可能にしてよいが MVP では固定値でよい。

---

# 15. Admin API key

`cx quota` は以下からのみ取得する。

```text
OPENAI_ADMIN_KEY
```

未設定時は request しない。

例。

```text
cx: OPENAI_ADMIN_KEY is required for `cx quota`

The Organization Usage API requires an OpenAI Admin API key.
The key is used only for this request and is not stored by cx.
```

通常の以下へ fallback しない。

```text
OPENAI_API_KEY
```

理由。

- Codex 用 Project API key と Administration API の credential を分離する
- accidental privilege escalation を防ぐ
- Admin key を明示的に利用していることを利用者に分かるようにする

---

# 16. Data Sharing 対象 project

Data Sharing が organization 全体で有効なら organization-wide scope でよい。

selected projects のみ有効にしている場合は `cx` に対象 project を指定する必要がある。

優先順位。

1. `--project`
2. `CX_OPENAI_PROJECT_IDS`
3. filter なし

環境変数。

```text
CX_OPENAI_PROJECT_IDS=proj_aaa,proj_bbb
```

selected projects を利用しているのに project scope が未設定の場合、正確な推定ができない可能性がある。

`--verbose` では必ず scope を表示する。

例。

```text
Scope: entire organization
```

または、

```text
Scope:
  proj_aaa
  proj_bbb
```

README には以下を書く。

```text
If Data Sharing is enabled only for selected projects,
configure those project IDs before relying on the estimate.
```

---

# 17. Usage Tier

公開 Usage API から complimentary token policy 用の現在の Usage Tier を確実に取得できることを前提にしない。

以下の優先順位とする。

1. `--usage-tier`
2. `CX_OPENAI_USAGE_TIER`
3. 既定値 `1`

環境変数。

```text
CX_OPENAI_USAGE_TIER
```

有効値。

```text
1
2
3
4
5
```

Tier 1 と Tier 2 は同一 quota。

Tier 3 から Tier 5 は同一 quota。

未指定の場合でも Tier 1 として quota と estimated remaining を計算する。Tier 1 を推測したのではなく、CLI の既定値として適用する。

Usage Tier を勝手に推測しない。

---

# 18. complimentary token policy

以下は 2026-09-06 時点の OpenAI Help Center を snapshot とした実装データである。

policy は将来変更される可能性があるため、1 箇所に集約する。

## 18.1 Large model group

Usage Tier 1 から 2。

```text
250,000 tokens/day
```

Usage Tier 3 から 5。

```text
1,000,000 tokens/day
```

対象モデル。

```text
gpt-5.6-sol
gpt-5.5-2026-04-23
gpt-5.4-2026-03-05
gpt-5.2-2025-12-11
gpt-5.1-2025-11-13
gpt-5.1-codex
gpt-5-codex
gpt-5-2025-08-07
gpt-5-chat-latest
gpt-4.5-preview-2025-02-27
gpt-4.1-2025-04-14
gpt-4o-2024-05-13
gpt-4o-2024-08-06
gpt-4o-2024-11-20
o3-2025-04-16
o1-preview-2024-09-12
o1-2024-12-17
```

`gpt-4.5-preview-2025-02-27` は Help Center 上に記載が残っているが deprecated and shut down と明記されている。

`gpt-5.6-sol` はこの group に属する。

## 18.2 Small model group

Usage Tier 1 から 2。

```text
2,500,000 tokens/day
```

Usage Tier 3 から 5。

```text
10,000,000 tokens/day
```

対象モデル。

```text
gpt-5.6-terra
gpt-5.6-luna
gpt-5.4-mini-2026-03-17
gpt-5.4-nano-2026-03-17
gpt-5.1-codex-mini
gpt-5-mini-2025-08-07
gpt-5-nano-2025-08-07
gpt-4.1-mini-2025-04-14
gpt-4.1-nano-2025-04-14
gpt-4o-mini-2024-07-18
o4-mini-2025-04-16
o1-mini-2024-09-12
codex-mini-latest
```

## 18.3 policy source

コードに snapshot date と source を残す。

```go
// Complimentary token policy snapshot: 2026-09-06
// Source: https://help.openai.com/en/articles/10306912
```

policy の自動 scraping はしない。

---

# 19. model classification

quota policy と model classification を 1 箇所へ集約する。

例。

```go
type QuotaPool string

const (
    QuotaPoolLarge QuotaPool = "large"
    QuotaPoolSmall QuotaPool = "small"
)
```

```go
func ClassifyModel(model string) (QuotaPool, bool)
```

未知 model を勝手にどちらかへ分類しない。

eligible ではない model は quota 計算から除外する。

`--verbose` では未知 model を確認できるようにする。

```text
Unclassified models:
  gpt-x.y-example
```

---

# 20. service tier の扱い

## 20.1 API から raw value を取得する

Organization Usage API は `service_tier` による grouping を正式にサポートしている。

したがって API には以下を指定する。

```text
group_by=service_tier
group_by=model
```

raw value は diagnostics として保持する。

## 20.2 raw value を complimentary 判定の必須根拠にしない

Dashboard 上では無料分が `data sharing incentive tier` と表示されるが、公開 API Reference は API の raw `service_tier` がどの literal になるかを明示していない。

したがって MVP の残量計算は raw tier の名前に依存させない。

`service_tier` は以下に使う。

- diagnostics
- `--verbose` 表示
- 将来、OpenAI が raw mapping を公式に文書化した場合の拡張点

## 20.3 verbose output

例。

```text
Observed API service tiers:
  default
  <raw-service-tier-value>
```

unknown tier があっても失敗しない。

---

# 21. quota 計算

各 pool について以下を計算する。

```text
eligible_traffic
quota_limit
estimated_remaining
estimated_usage_percent
```

## 21.1 eligible traffic

指定された Data Sharing scope 内で、eligible model に対して発生した、

```text
input_tokens + output_tokens
```

の当日合計。

## 21.2 estimated remaining

Usage Tier が分かっている場合。

```text
estimated_remaining = max(0, quota_limit - eligible_traffic)
```

ただしこの値は conservative estimate とする。

以下の理由で Dashboard 値とずれる可能性がある。

- project scope の設定ミス
- Data Sharing configuration が API から取得できない
- request boundary による quota crossing
- policy 更新
- Usage API 集計遅延
- API の model naming change
- complimentary program eligibility の変更

## 21.3 Dashboard incentive usage は別概念

将来、OpenAI が API 上の incentive tier mapping を公式に文書化した場合のみ、

```text
confirmed complimentary usage
```

のような項目を追加してよい。

MVP では Dashboard にしか公式に説明されていない表示名を API response へ無理に対応づけない。

---

# 22. `cx quota` 標準出力

例。

```text
OpenAI complimentary token estimate
Window: 2026-09-06 00:00 UTC - 2026-09-06 03:20 UTC
Usage Tier: 2
Policy snapshot: 2026-09-06
Scope: entire organization

Large model group
  quota:                 250,000
  eligible traffic:      183,421
  estimated remaining:    66,579
  estimated usage:          73.4%
  includes: gpt-5.6-sol

Small model group
  quota:               2,500,000
  eligible traffic:       12,500
  estimated remaining: 2,487,500
  estimated usage:           0.5%

Next reset: 2026-09-07 00:00 UTC

Note:
  Remaining values are estimates derived from the Organization Usage API
  and the published complimentary-token policy.
  Confirm actual complimentary usage in the OpenAI Usage Dashboard.
```

## 22.1 quota 超過例

```text
Large model group
  quota:                 250,000
  eligible traffic:      281,000
  estimated remaining:         0
  status:                quota likely exhausted
```

`exhausted` と断定せず、必要に応じて `likely exhausted` と表現する。

## 22.2 Usage Tier の既定値

```text
OpenAI complimentary token estimate
Usage Tier: 1
Policy snapshot: 2026-09-06

Large model group
  eligible traffic: 183,421
  quota: 250,000
  estimated remaining: 66,579

Set CX_OPENAI_USAGE_TIER or use --usage-tier to override the default.
```

---

# 23. Dashboard 案内

`cx quota --verbose` の末尾に公式確認方法を出してよい。

```text
Official verification:
  https://platform.openai.com/usage/chat-completions

In the dashboard:
  1. Enable input tokens and output tokens
  2. Group by Service tier
  3. Check "data sharing incentive tier" usage
```

通常出力では URL のみでもよい。

---

# 24. JSON output

```bash
cx quota --json
```

stdout は JSON のみにする。

例。

```json
{
  "kind": "estimate",
  "window": {
    "start": "2026-09-06T00:00:00Z",
    "end": "2026-09-06T03:20:00Z",
    "next_reset": "2026-09-07T00:00:00Z"
  },
  "usage_tier": 2,
  "scope": {
    "project_ids": []
  },
  "pools": {
    "large": {
      "quota": 250000,
      "eligible_traffic": 183421,
      "estimated_remaining": 66579,
      "estimated_usage_percent": 73.3684
    },
    "small": {
      "quota": 2500000,
      "eligible_traffic": 12500,
      "estimated_remaining": 2487500,
      "estimated_usage_percent": 0.5
    }
  },
  "official_verification_url": "https://platform.openai.com/usage/chat-completions",
  "warnings": []
}
```

Tier 不明時。

```json
{
  "quota": null,
  "estimated_remaining": null
}
```

`kind` は必ず、

```text
estimate
```

とする。

将来 official remaining API が追加されない限り `kind=official` は使わない。

---

# 25. verbose output

`cx quota --verbose` では以下も表示する。

- API query time range
- project scope
- Usage Tier source
- model 別集計
- raw service tier 別集計
- unclassified models
- policy snapshot date
- official Dashboard URL
- warnings

Admin key は絶対に表示しない。

---

# 26. HTTP error handling

## 401

```text
cx: OpenAI Usage API authentication failed
Check OPENAI_ADMIN_KEY.
```

## 403

```text
cx: OPENAI_ADMIN_KEY does not have permission to read organization usage
Use an appropriate Organization Admin API key.
```

## 429

```text
cx: OpenAI Usage API rate limit exceeded
```

## 5xx

```text
cx: OpenAI Usage API is temporarily unavailable
```

timeout。

```text
cx: OpenAI Usage API request timed out
```

response body をそのまま無制限に表示しない。

safe な HTTP status と request ID が取得できる場合は表示してよい。

---

# 27. HTTP client requirements

以下を満たす。

- HTTPS のみ
- timeout あり
- context 対応
- pagination 対応
- response body size を無制限に信用しない
- non-2xx を明示的に処理
- Admin key を query parameter に入れない
- Admin key を log に出さない
- User-Agent を付ける

例。

```text
User-Agent: cx/<version>
```

---

# 28. profile directory

対象 profile の directory が存在しなければ作成する。

Unix 系。

```go
os.MkdirAll(profileDir, 0700)
```

Windows ACL を独自操作する必要は MVP ではない。

---

# 29. child process

Codex process は interactive CLI として正常に動作する必要がある。

```go
cmd.Stdin = os.Stdin
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
```

Codex child process の exit code を可能な限りそのまま返す。

例。

Codex が 42 で終了したら `cx` も 42。

OS ごとの signal handling を過剰に抽象化しない。

---

# 30. config.toml

MVP では `chatgpt` と `api` の `config.toml` を自動同期しない。

理由。

- Codex config format へ依存しすぎない
- profile ごとに model 等を変更したい可能性がある
- MVP の責務を profile switching に限定する

将来必要なら、

```text
cx config sync
```

を検討する。

Windows portability のため symlink 前提にしない。

---

# 31. command grammar

```text
cx <profile> [codex args...]

cx login <profile>
cx status [profile]
cx path <profile>
cx profiles

cx quota [options]

cx help
```

reserved commands。

```text
login
status
path
profiles
quota
help
```

unknown command。

```text
cx: unknown command or profile: foo
```

exit code 2。

---

# 32. package structure

```text
codex-profile-switcher/
├── cmd/
│   └── cx/
│       └── main.go
├── internal/
│   ├── app/
│   │   └── app.go
│   ├── profile/
│   │   └── profile.go
│   ├── codex/
│   │   └── runner.go
│   └── openaiusage/
│       ├── client.go
│       ├── types.go
│       ├── policy.go
│       ├── quota.go
│       └── report.go
├── go.mod
├── README.md
└── LICENSE
```

package を過剰に細分化しない。

Codex runner、HTTP client、quota calculation は分離する。

---

# 33. suggested interfaces

Codex。

```go
type CodexRunner interface {
    Run(ctx context.Context, profile Profile, args []string) error
    Login(ctx context.Context, profile Profile) error
    LoginStatus(ctx context.Context, profile Profile) error
}
```

Usage。

```go
type UsageClient interface {
    CompletionUsage(
        ctx context.Context,
        query UsageQuery,
    ) ([]CompletionUsageResult, error)
}
```

quota calculation。

```go
func CalculateEstimate(
    results []CompletionUsageResult,
    policy ComplimentaryPolicy,
    usageTier *int,
) QuotaReport
```

HTTP と計算ロジックを分離する。

---

# 34. tests

## 34.1 profile tests

最低限。

- default profile root
- `CX_HOME`
- chatgpt path
- api path
- invalid profile
- filepath handling

## 34.2 Codex runner tests

fake executable を使用する。

確認事項。

- child process の `CODEX_HOME`
- credential 環境変数を child process へ継承しない
- parent environment を変更しない
- arguments の透過
- stdin stdout stderr
- exit code
- `login chatgpt`
- `login api --with-api-key`

実 credential をテストで触らない。

## 34.3 Usage API tests

`httptest.Server` を利用する。

最低限。

- Authorization header
- start_time
- end_time
- bucket_width
- model grouping
- service_tier grouping
- project filter
- pagination
- 401
- 403
- 429
- 500
- timeout
- malformed JSON
- response size handling

実 OpenAI API は unit test から呼ばない。

## 34.4 quota calculation tests

### Tier 2 Large

```text
quota = 250000
traffic = 100000
estimated remaining = 150000
```

### Tier 4 Large

```text
quota = 1000000
traffic = 100000
estimated remaining = 900000
```

### Tier 2 Small

```text
quota = 2500000
```

### Tier 4 Small

```text
quota = 10000000
```

### over quota

```text
traffic = 281000
quota = 250000
estimated remaining = 0
```

### cached token

`input_cached_tokens` を追加加算しない。

### unknown model

既存 pool へ勝手に分類しない。

### unknown service tier

クラッシュしない。

### unknown Usage Tier

`CalculateEstimate` に nil を渡した場合は traffic を算出するが quota と remaining は null。CLI は nil を渡さず、未指定時は Tier 1 を使う。

### selected projects

指定 project 以外を計算対象にしないことを API query または fixture で検証する。

---

# 35. security requirements

必須。

- Codex `auth.json` を解析しない
- Codex API key を保存しない
- Admin API key を保存しない
- Codex child process へ `OPENAI_ADMIN_KEY`、`OPENAI_API_KEY`、`CODEX_API_KEY`、`CODEX_ACCESS_TOKEN` を継承しない
- secret を CLI args に要求しない
- secret を stdout に出さない
- secret を stderr に出さない
- secret を debug log に出さない
- Admin key は Authorization header にのみ使用する
- `OPENAI_ADMIN_KEY` 未設定時は API request しない
- `OPENAI_API_KEY` に fallback しない
- OpenAI Dashboard private endpoint を使用しない

test で dummy secret が output に含まれないことを確認する。

---

# 36. README requirements

README に最低限以下を書く。

## Install

`go install`。

release binary。

## Initial setup

ChatGPT account authentication。

```bash
cx login chatgpt
```

API。

```bash
printenv OPENAI_API_KEY | cx login api
```

PowerShell。

```powershell
$env:OPENAI_API_KEY | cx login api
```

API key は stdin から渡し、入力処理と credential 保存は Codex に委譲されることを明記する。`cx login api` 単独実行は TTY からの入力を拒否して pipe の形式を案内する。

## Launch

```bash
cx chatgpt
cx api
```

## Quota estimate

```bash
export OPENAI_ADMIN_KEY="..."
export CX_OPENAI_USAGE_TIER=2
cx quota
```

PowerShell。

```powershell
$env:OPENAI_ADMIN_KEY = "..."
$env:CX_OPENAI_USAGE_TIER = "2"
cx quota
```

## Selected projects

```bash
export CX_OPENAI_PROJECT_IDS="proj_aaa,proj_bbb"
cx quota
```

## Important limitation

明確に以下を書く。

```text
cx quota does not read an official complimentary-token balance.
It estimates remaining quota from the Organization Usage API and
OpenAI's published complimentary-token policy.

For authoritative verification of complimentary token usage, use
the OpenAI Usage Dashboard and group Chat Completions by Service tier.
```

## Official Dashboard

```text
https://platform.openai.com/usage/chat-completions
```

## Admin API key security

Admin key は通常の Project API key より強い organization-level permission を持つため、取り扱いに注意する。

---

# 37. exit codes

基本。

```text
0 success
1 runtime failure
2 invalid CLI usage
```

Codex launch mode では child process の exit code を可能な限り透過する。

`cx quota` では API error や parse error は 1。

推定残量 0 は正常な状態なので 0。

---

# 38. acceptance criteria

MVP 完了条件。

- `go build ./...` 成功
- `go test ./...` 成功
- Windows、Linux、macOS を考慮した path 実装
- `cx chatgpt` が chatgpt profile の `CODEX_HOME` を使う
- `cx api` が api profile の `CODEX_HOME` を使う
- chatgpt と api の Codex 認証状態が独立する
- `cx login chatgpt` が Codex login を実行する
- `cx login api` が `codex login --with-api-key` を実行する
- `cx login api` が TTY から直接実行された場合に stdin pipe の形式を案内する
- Codex arguments をそのまま透過できる
- `cx status` が profile 別 status を確認する
- `cx path` が script-friendly output を返す
- `cx quota` が Organization Usage API を利用する
- `cx quota` が 00:00 UTC から現在までを取得する
- `cx quota` が Large と Small の model group を分離する
- input と output tokens を合計する
- cached tokens を二重加算しない
- Usage Tier 1-2 と 3-5 の quota 差を扱える
- selected project scope を扱える
- unknown model を勝手に quota group へ入れない
- unknown service tier で壊れない
- `cx quota --json` が machine-readable
- JSON に `kind: estimate` が入る
- output が推定値であることを明示する
- official Dashboard URL を README に記載する
- Admin API key が output に露出しない
- Codex child process に credential 環境変数を継承しない
- `auth.json` を直接操作しない
- private Dashboard API を使用しない
- unit test から実 OpenAI API を呼ばない

---

# 39. 実装優先順位

## Phase 1

profile switching。

- profile path
- `cx chatgpt`
- `cx api`
- argument passthrough
- process exit code
- `CX_HOME`
- `CX_CODEX_BIN`

## Phase 2

profile operations。

- `login`
- `status`
- `path`
- `profiles`
- help

## Phase 3

Usage API。

- Admin API key
- Organization Usage API client
- UTC time range
- pagination
- model grouping
- service tier grouping
- project filter
- raw usage model

## Phase 4

quota estimate。

- complimentary policy snapshot
- model classification
- Usage Tier
- pool aggregation
- estimated remaining
- conservative wording
- Dashboard guidance

## Phase 5

operability。

- `--json`
- `--verbose`
- README
- cross compile
- release build

各 Phase で build と test を通す。

---

# 40. Codex への実装指示

この仕様を実装する際は、まず repository 内の既存コードを確認し、既存構造がある場合はそれを尊重すること。

新規 repository なら最小構成から開始する。

実装原則。

- KISS
- YAGNI
- secret の独自保存機構を作らない
- Codex credential internals に依存しない
- OpenAI の undocumented API を使わない
- OpenAI Dashboard を scrape しない
- quota estimate と authoritative data を混同しない
- HTTP access と計算ロジックを分離する
- OS 固有コードを必要最小限にする
- testability を優先する
- 不要な framework を追加しない

実装後。

```bash
go fmt ./...
go vet ./...
go test ./...
go build ./...
```

可能なら cross compile。

```bash
GOOS=windows GOARCH=amd64 go build ./cmd/cx
GOOS=linux GOARCH=amd64 go build ./cmd/cx
GOOS=darwin GOARCH=arm64 go build ./cmd/cx
```

実 Codex login や実 OpenAI Admin API を CI から実行しない。

最後に報告する。

- 実装 command
- file structure
- tests
- security considerations
- platform-specific limitations
- quota estimate の制約
- 未実装事項
- 実 API と Dashboard で手動確認すべき項目

---

# 41. 手動検証項目

実装後、利用者が実環境で以下を確認する。

## 41.1 Codex profile

```bash
cx login chatgpt
cx status chatgpt

printenv OPENAI_API_KEY | cx login api
cx status api
```

認証が独立していること。

## 41.2 Usage API

```bash
cx quota --verbose
```

API access が成功すること。

## 41.3 Dashboard との比較

同じ UTC 日について、

```text
https://platform.openai.com/usage/chat-completions
```

を開き、

- input tokens
- output tokens

を有効にする。

Group by。

```text
Service tier
```

を選ぶ。

Dashboard の、

```text
data sharing incentive tier
```

と `cx quota` の eligible traffic や推定結果を比較する。

差異がある場合は API response の raw service tier、project scope、policy を調査する。

この検証で API raw `service_tier` と Dashboard の incentive tier の対応が安定して確認できても、OpenAI 公式 docs に literal mapping が掲載されるまでは hard-coded official mapping として扱わない。

---

# 42. 公式仕様参照

実装開始時と release 前に必ず最新仕様を確認する。

## Data Sharing と complimentary daily tokens

```text
https://help.openai.com/en/articles/10306912
```

2026-09-06 時点で重要な記載。

- 対象 organization のみ complimentary tokens を利用できる
- Data Sharing を有効化した traffic に自動適用される
- selected projects の場合は対象 project の traffic のみ適用される
- positive account balance が必要
- Usage と Costs の比較で free usage を確認できる
- Chat Completions Dashboard を Service tier で group すると確認できる
- Dashboard 上では `data sharing incentive tier - input/output tokens`
- Large group は Tier 1-2 で 250K、Tier 3-5 で 1M tokens/day
- Small group は Tier 1-2 で 2.5M、Tier 3-5 で 10M tokens/day
- quota は model group 内で共有
- quota crossing request は request 全体が billed
- counter は 00:00 UTC に reset

## Organization Usage API

```text
https://developers.openai.com/api/reference/ruby/resources/admin/subresources/organization/subresources/usage/methods/completions
```

重要な公開仕様。

- `GET /organization/usage/completions`
- `start_time`
- `end_time`
- `bucket_width`
- `project_ids`
- `models`
- `page`
- `group_by`
- `group_by` に `model` と `service_tier` を含められる
- result に `input_tokens`
- result に `output_tokens`
- result に `input_cached_tokens`
- result に `model`
- result に `service_tier`
- `has_more`
- `next_page`
- 公式サンプルは Admin API key を使用

## Admin API Keys

```text
https://developers.openai.com/api/reference/python/resources/admin/subresources/organization/subresources/admin_api_keys
```

---

# 43. MVP 完了後に検討可能な機能

MVP には含めない。

## 43.1 `cx quota open`

OS の標準 browser で公式 Usage Dashboard を開く。

```bash
cx quota open
```

これを実装する場合も scraping はしない。

単に、

```text
https://platform.openai.com/usage/chat-completions
```

を開くだけとする。

## 43.2 configurable policy

OpenAI が quota policy を変更した場合に binary update なしで設定できる仕組み。

ただし remote HTML parsing は行わない。

## 43.3 officially documented incentive tier mapping

将来 OpenAI API Reference が raw `service_tier` と `data sharing incentive tier` の対応を正式に公開した場合のみ、

```text
confirmed complimentary usage
```

を API から算出する機能を追加する。

それまでは `estimated` のままとする。

---

# 44. 最終的な設計意図

`cx` の責務は二つに限定する。

第一。

```text
Codex の認証環境を CODEX_HOME で安全に切り替える
```

第二。

```text
公開 OpenAI Usage API と公開 policy を使って
complimentary token の利用状況を CLI から推定しやすくする
```

OpenAI Platform が持つ billing や complimentary token の authoritative state を `cx` が再実装しようとしてはならない。

公式確認先は常に OpenAI Usage Dashboard とする。

この境界を守ることで、Codex や OpenAI Platform の内部実装変更に対して壊れにくく、安全で小さい cross-platform CLI とする。
