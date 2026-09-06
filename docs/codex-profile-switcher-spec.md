# Codex Profile Switcher `cx` 実装仕様書

更新日: 2026-09-06

## 1. 目的

OpenAI Codex CLI の実行環境をプロファイルごとに分離し、ChatGPT 認証と OpenAI API キー認証を簡単かつ安全に切り替えて利用できるクロスプラットフォーム CLI を Go で実装する。

ツール名は `cx` とする。

主な用途は以下。

- ChatGPT Plus の Codex 利用枠を使う
- OpenAI API キー認証へ切り替えて API 側の利用枠を使う
- Data Sharing Incentive の complimentary daily tokens を利用する
- complimentary token の当日使用量と推定残量を確認する
- Windows、WSL、macOS で同じ操作体系を使う
- 将来的に `local`、`work` など追加の Codex プロファイルへ拡張可能にする

---

## 2. 最重要設計方針

### 2.1 認証ファイルを直接切り替えない

`cx` は以下を行ってはならない。

- Codex の `auth.json` をコピーする
- Codex の `auth.json` を編集する
- Codex の `auth.json` を解析する
- ChatGPT OAuth token を直接扱う
- Codex 用 API key を直接保存する

代わりに、プロファイルごとに異なる `CODEX_HOME` を設定して Codex CLI を子プロセスとして起動する。

```text
<UserHome>/.codex-profiles/
├── plus/
│   ├── auth.json
│   └── config.toml
└── api/
    ├── auth.json
    └── config.toml
```

Codex 自身に credential storage と token refresh を任せる。

### 2.2 `cx` はランチャーとして振る舞う

例えば、

```bash
cx plus
```

は概念的に以下と同等。

```bash
CODEX_HOME="$HOME/.codex-profiles/plus" codex
```

```bash
cx api
```

は概念的に以下と同等。

```bash
CODEX_HOME="$HOME/.codex-profiles/api" codex
```

Go では環境変数を子プロセスのみに設定する。

例。

```go
cmd := exec.Command(codexBin, args...)
cmd.Env = append(os.Environ(), "CODEX_HOME="+profileDir)
```

親シェルの `CODEX_HOME` は変更しない。

### 2.3 認証処理は Codex 本体へ委譲する

ChatGPT 認証。

```text
CODEX_HOME=<plus-dir> codex login
```

API key 認証。

```text
CODEX_HOME=<api-dir> codex login --with-api-key
```

`cx` 自身は認証 credential の内部形式に依存しない。

### 2.4 complimentary token 確認用 credential は Codex credential と分離する

OpenAI Organization Usage API は Organization Admin API key を必要とする。

これは Codex がモデル呼び出しに利用する Project API key とは別物として扱う。

MVP では Admin API key を保存しない。

以下の環境変数からのみ取得する。

```text
OPENAI_ADMIN_KEY
```

`cx` は値をログ、エラー、debug output に出してはならない。

---

## 3. 対応プラットフォーム

最低限、以下を対象とする。

- Windows amd64
- Windows arm64
- Linux amd64
- Linux arm64
- macOS amd64
- macOS arm64

WSL は Linux として扱う。

Go のクロスコンパイルで release binary を生成可能にする。

---

## 4. 技術方針

MVP は Go 標準ライブラリのみで実装する。

CLI framework は使用しない。

利用候補。

- `os`
- `os/exec`
- `path/filepath`
- `fmt`
- `errors`
- `strings`
- `net/http`
- `net/url`
- `encoding/json`
- `time`
- `context`

Cobra、Viper、OpenAI SDK などは MVP では導入しない。

理由は以下。

- CLI が小さい
- OpenAI Usage API は単純な HTTP GET で利用できる
- binary を軽量に保つ
- dependency surface を小さくする

---

## 5. 非目標

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
- OpenAI の billing 設定変更
- Usage Tier の自動変更
- OpenAI の非公開 API や Dashboard 内部 API の利用
- HTML scraping
- ブラウザ automation

公開されている OpenAI API のみ利用する。

---

## 6. CLI 名

実行ファイル。

```text
cx
```

リポジトリ名候補。

```text
codex-profile-switcher
```

Go module は実際に作成した GitHub repository に合わせる。

---

## 7. デフォルトプロファイル

MVP では以下の 2 プロファイルを組み込みで提供する。

| profile | purpose |
| --- | --- |
| `plus` | ChatGPT login |
| `api` | OpenAI API key login |

将来的に以下を追加可能な内部設計にする。

```text
local
work
test
```

ただし MVP では任意プロファイル追加コマンドは不要。

---

## 8. プロファイル保存場所

デフォルト。

```text
<UserHome>/.codex-profiles
```

Go では `os.UserHomeDir()` を利用する。

各 profile の `CODEX_HOME`。

```text
<UserHome>/.codex-profiles/<profile>
```

Linux、WSL、macOS 例。

```text
/home/user/.codex-profiles/plus
/home/user/.codex-profiles/api
```

Windows 例。

```text
C:\Users\user\.codex-profiles\plus
C:\Users\user\.codex-profiles\api
```

### 8.1 `CX_HOME`

以下の環境変数で profile root を上書き可能にする。

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

### 8.2 `CX_CODEX_BIN`

Codex executable は通常、

```go
exec.LookPath("codex")
```

で探索する。

以下が指定されている場合は優先する。

```text
CX_CODEX_BIN
```

例。

```bash
CX_CODEX_BIN=/opt/codex/bin/codex cx api
```

---

# 9. CLI 仕様

## 9.1 Codex 起動

```text
cx plus [codex args...]
cx api [codex args...]
```

例。

```bash
cx plus
cx api
```

Codex arguments は原則としてそのまま透過する。

```bash
cx api exec "このPRをレビューして"
```

内部的には、

```text
CODEX_HOME=<api-dir> codex exec "このPRをレビューして"
```

を実行する。

以下も動作させる。

```bash
cx api --model gpt-5.6-sol
cx plus --model gpt-5.6-sol
```

`--` の挿入を必須にしない。

---

## 9.2 login

```text
cx login plus
cx login api
```

### plus

```bash
cx login plus
```

内部では、

```text
CODEX_HOME=<plus-dir> codex login
```

を実行する。

### api

```bash
cx login api
```

内部では、

```text
CODEX_HOME=<api-dir> codex login --with-api-key
```

を実行する。

stdin、stdout、stderr は Codex process へ接続する。

`cx` が API key を読み取ったり保存したりしてはならない。

以下のような CLI option は作らない。

```text
cx login api --key sk-...
```

shell history や process list に secret が露出するため禁止する。

---

## 9.3 status

全 profile。

```bash
cx status
```

個別。

```bash
cx status plus
cx status api
```

各 profile に対して、

```text
CODEX_HOME=<profile-dir> codex login status
```

を実行して状態を取得する。

`auth.json` を直接解析してはならない。

想定出力。

```text
plus:
  Logged in using ChatGPT

api:
  Logged in using an API key
```

Codex の status command が失敗した場合も、他 profile の確認は続行する。

---

## 9.4 path

```bash
cx path plus
cx path api
```

stdout には path のみ表示する。

例。

```text
/home/user/.codex-profiles/api
```

スクリプトから安全に利用できるよう、装飾を付けない。

---

## 9.5 profiles

```bash
cx profiles
```

MVP。

```text
plus
api
```

---

## 9.6 help

以下をサポートする。

```text
cx
cx help
cx --help
cx -h
```

引数なしは help を表示して exit code 0。

---

# 10. complimentary token 使用量確認

## 10.1 command

以下を追加する。

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

Usage Tier を明示。

```bash
cx quota --usage-tier 2
cx quota --usage-tier 4
```

対象 project を限定。

```bash
cx quota --project proj_xxx
```

複数指定可能。

```bash
cx quota --project proj_xxx --project proj_yyy
```

---

## 10.2 使用 API

OpenAI Organization Usage API を利用する。

```text
GET https://api.openai.com/v1/organization/usage/completions
```

Authorization。

```text
Authorization: Bearer $OPENAI_ADMIN_KEY
```

最低限以下を指定する。

```text
start_time=<today 00:00 UTC>
end_time=<current time>
bucket_width=1d
group_by=service_tier
group_by=model
```

必要に応じて、

```text
project_ids[]=proj_xxx
```

を付与する。

API pagination の `has_more` と `next_page` を正しく処理する。

HTTP timeout を設定する。

推奨。

```text
10 seconds
```

---

## 10.3 Admin API key

`cx quota` は以下を読む。

```text
OPENAI_ADMIN_KEY
```

未設定の場合は API request を行わず、明確なエラーを出す。

例。

```text
cx: OPENAI_ADMIN_KEY is required for `cx quota`
Create an Organization Admin API key in the OpenAI Platform and expose it through the OPENAI_ADMIN_KEY environment variable.
```

secret 自体は絶対に表示しない。

通常の `OPENAI_API_KEY` への fallback は行わない。

理由。

- Usage endpoint は organization-level endpoint
- Codex 用 Project API key と権限が異なる
- privilege boundary を明確に保つ

---

## 10.4 complimentary token は直接残量 API があるわけではない

OpenAI の公開 API は complimentary token の `remaining` 値そのものを返す仕様ではない。

そのため `cx quota` は Usage API の実績値から推定する。

出力では、

```text
remaining
```

ではなく、

```text
estimated remaining
```

と表記する。

---

# 11. complimentary token quota のルール

2026-09-06 時点の OpenAI 公開仕様を実装基準とする。

complimentary token は 2 つの shared quota pool に分かれる。

## 11.1 Large model pool

Usage Tier 1 から 2。

```text
250,000 tokens/day
```

Usage Tier 3 から 5。

```text
1,000,000 tokens/day
```

2026-09-06 時点の対象モデル。

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
gpt-4.1-2025-04-14
gpt-4o-2024-05-13
gpt-4o-2024-08-06
gpt-4o-2024-11-20
o3-2025-04-16
o1-preview-2024-09-12
o1-2024-12-17
```

`gpt-5.6-sol` はこの pool に属する。

## 11.2 Small model pool

Usage Tier 1 から 2。

```text
2,500,000 tokens/day
```

Usage Tier 3 から 5。

```text
10,000,000 tokens/day
```

2026-09-06 時点の対象モデル。

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

---

# 12. reset 時刻

complimentary token counter は毎日、

```text
00:00 UTC
```

に reset される。

したがって `cx quota` の集計期間は local timezone ではなく UTC で決定する。

Go 例。

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

東京では通常、

```text
09:00 JST
```

が reset 時刻になる。

表示例。

```text
Reset: 2026-09-07 00:00 UTC / 2026-09-07 09:00 JST
```

local timezone の表示は `time.Local` を使用してよい。

---

# 13. Usage Tier の扱い

公開 Usage API から organization の現在 Usage Tier を安定して直接取得できることを前提にしてはならない。

そのため以下の優先順位で決定する。

1. `--usage-tier`
2. `CX_OPENAI_USAGE_TIER`
3. 未指定

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

Tier 1 と 2 は同じ complimentary quota。

Tier 3 から 5 も同じ complimentary quota。

### Tier 未指定時

使用量は表示するが、推定残量は計算しない。

例。

```text
Complimentary usage today

Large model pool
  used complimentary: 183,421 tokens
  quota:               unknown
  estimated remaining: unknown

Set CX_OPENAI_USAGE_TIER or use --usage-tier to calculate the quota.
```

Usage Tier を勝手に推測してはならない。

---

# 14. Data Sharing 対象 project

Data Sharing が organization 全体で有効なら project filter は不要。

`Enabled for selected projects` を使う organization では、対象 project のみ quota 集計対象にする必要がある。

以下の優先順位で project IDs を決める。

1. `--project`
2. `CX_OPENAI_PROJECT_IDS`
3. filter なし

環境変数は comma separated とする。

```text
CX_OPENAI_PROJECT_IDS=proj_aaa,proj_bbb
```

例。

```bash
cx quota --project proj_aaa --usage-tier 2
```

project filter が未指定の場合は organization 全体を問い合わせる。

ただし、選択 project のみ Data Sharing を有効にしている環境では organization-wide usage を quota の running total と見なすと過大集計になる可能性がある。

そのため `--verbose` では scope を明示する。

```text
Scope: entire organization
```

または、

```text
Scope:
  proj_aaa
  proj_bbb
```

---

# 15. `service_tier` の扱い

Usage API は `group_by=service_tier` をサポートする。

OpenAI Dashboard では complimentary token が、

```text
data sharing incentive tier
```

として表示される。

ただし API の raw `service_tier` string の表現は将来変更される可能性があるため、コード全体で undocumented literal に強く依存しない。

実装では以下を分離する。

```go
func IsDataSharingIncentiveTier(serviceTier string) bool
```

この関数に判定を集約する。

未知の `service_tier` は捨てず、`--verbose` で raw value を表示する。

recognized incentive tier が 0 件でも Usage API 自体が成功している場合はエラー扱いにしない。

例。

```text
No data-sharing incentive usage was observed in the selected time range.
```

`--verbose`。

```text
Observed service tiers:
  default
  <raw value>
```

これにより API 側の naming 変更を調査可能にする。

---

# 16. token 集計

Completions Usage result の以下を利用する。

```text
input_tokens
output_tokens
model
service_tier
project_id
```

基本 token 数。

```text
total_tokens = input_tokens + output_tokens
```

同一 token を二重加算しないこと。

`input_cached_tokens` は `input_tokens` に含まれるため別途加算しない。

audio token 等、complimentary program の対象外または仕様が不明なものを勝手に加算しない。

---

# 17. quota 計算

各 pool ごとに以下を計算する。

```text
complimentary_used
eligible_traffic_total
quota_limit
estimated_remaining
```

### 17.1 complimentary_used

data sharing incentive `service_tier` と識別された record の、

```text
input_tokens + output_tokens
```

を合計する。

### 17.2 eligible_traffic_total

選択 scope 内で、その pool の対象 model に対して発生した全 service tier の、

```text
input_tokens + output_tokens
```

を合計する。

これは quota 上限をまたいだ request が丸ごと通常課金になったケースを検出するために利用する。

### 17.3 estimated_remaining

Usage Tier が既知の場合。

```text
if eligible_traffic_total >= quota_limit:
    estimated_remaining = 0
else:
    estimated_remaining = quota_limit - eligible_traffic_total
```

ただしこれは推定値である。

OpenAI は各 request 開始時点で running total と request size を確認し、1 request が quota を超過する場合はその request 全体を通常課金する。

そのため、

```text
estimated_remaining = 20,000
```

であっても、次の request が 25,000 tokens なら無料になる保証はない。

出力にこの性質を明記する。

---

# 18. `cx quota` 標準出力

例。

```text
OpenAI complimentary token usage
Window: 2026-09-06 00:00 UTC - 2026-09-06 03:20 UTC
Usage Tier: 2
Scope: entire organization

Large model pool
  quota:                 250,000
  complimentary used:   183,421
  eligible traffic:     183,421
  estimated remaining:   66,579
  usage:                  73.4%

Small model pool
  quota:               2,500,000
  complimentary used:     12,500
  eligible traffic:       12,500
  estimated remaining: 2,487,500
  usage:                    0.5%

Next reset: 2026-09-07 00:00 UTC
```

quota を使い切った例。

```text
Large model pool
  quota:                 250,000
  complimentary used:   247,000
  eligible traffic:     281,000
  estimated remaining:        0
  status:                quota exceeded
```

このケースで `complimentary used` が quota より少なくても `eligible traffic` が上限を超えているため残量 0 とする。

---

# 19. Codex 用表示

`gpt-5.6-sol` が主目的なので、標準出力の Large model pool に以下を付けてもよい。

```text
Includes: gpt-5.6-sol
```

ただし model-specific quota と誤解させないこと。

quota は pool 内モデル間で共有される。

以下のような表示は禁止。

```text
gpt-5.6-sol quota: 250,000
```

正しくは、

```text
Large model pool quota: 250,000
Includes gpt-5.6-sol and other eligible models
```

---

# 20. JSON output

```bash
cx quota --json
```

machine-readable な JSON を stdout に出す。

例。

```json
{
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
      "complimentary_used": 183421,
      "eligible_traffic": 183421,
      "estimated_remaining": 66579,
      "includes_gpt_5_6_sol": true
    },
    "small": {
      "quota": 2500000,
      "complimentary_used": 12500,
      "eligible_traffic": 12500,
      "estimated_remaining": 2487500
    }
  }
}
```

Tier 未指定時は、

```json
"quota": null,
"estimated_remaining": null
```

とする。

JSON mode では human-readable text を stdout に混ぜない。

warning は stderr に出すか JSON の `warnings` field に含める。

---

# 21. API error handling

以下を区別して表示する。

## 401

```text
cx: OpenAI Usage API authentication failed
Check OPENAI_ADMIN_KEY.
```

## 403

```text
cx: OPENAI_ADMIN_KEY does not have permission to read organization usage
An Organization Owner Admin API key is required.
```

## 429

```text
cx: OpenAI Usage API rate limit exceeded
```

## 5xx

```text
cx: OpenAI Usage API is temporarily unavailable
```

network timeout。

```text
cx: OpenAI Usage API request timed out
```

response body に secret が含まれる可能性を考慮し、無制限に body をエラー表示しない。

必要なら safe な request ID と status code のみ表示する。

---

# 22. HTTP client requirements

以下を満たす。

- HTTPS のみ
- timeout あり
- context 対応
- pagination 対応
- response body size を無制限に信用しない
- non-2xx を明示的に処理
- Admin key を URL query に入れない
- Admin key をログに出さない
- User-Agent を付ける

例。

```text
User-Agent: cx/<version>
```

OpenAI SDK の導入は不要。

---

# 23. profile directory 作成

対象 profile を初めて使う際、存在しなければ作成する。

Unix 系では可能な限り owner only permission とする。

```go
os.MkdirAll(profileDir, 0700)
```

Windows の ACL を独自操作する必要は MVP ではない。

---

# 24. child process

Codex process は interactive CLI として正常に動作する必要がある。

最低限以下を接続する。

```go
cmd.Stdin = os.Stdin
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
```

signal と exit code の扱いも可能な限り透過する。

Codex が exit code 42 で終了した場合、`cx` も原則 42 で終了する。

Windows と Unix の signal model 差異を考慮し、過剰な process management はしない。

---

# 25. config.toml

MVP では `plus` と `api` の `config.toml` を自動同期しない。

理由。

- Codex 側の config format に依存したくない
- profile ごとに設定を変えたいケースもある
- MVP の責務を profile switching に限定する

必要になった場合にのみ将来、

```text
cx config sync
```

などを検討する。

symlink 前提にはしない。

Windows での portability を優先する。

---

# 26. command parser

MVP grammar。

```text
cx <profile> [codex args...]

cx login <profile>
cx status [profile]
cx path <profile>
cx profiles
cx quota [options]
cx help
```

reserved top-level commands。

```text
login
status
path
profiles
quota
help
```

それ以外の第一引数が valid profile なら Codex launch とする。

不明な command。

```text
cx: unknown command or profile: foo
```

exit code。

```text
2
```

---

# 27. recommended package structure

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
│       ├── quota.go
│       └── models.go
├── go.mod
├── README.md
└── LICENSE
```

package boundary を過剰に細分化しない。

ただし OpenAI Usage API client と Codex process launcher は分離する。

---

# 28. suggested internal interfaces

例。

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

quota calculation は pure function に近づける。

```go
func CalculateQuota(
    results []CompletionUsageResult,
    usageTier int,
) QuotaReport
```

HTTP と計算ロジックを分離し、fixture test を容易にする。

---

# 29. model pool classification

model classification は 1 箇所に集約する。

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

未知 model を勝手に既存 pool へ入れない。

未知 model が incentive service tier として返った場合は warning を出す。

例。

```text
warning: unknown complimentary-token model observed: gpt-x.y
```

これにより OpenAI が対象 model を追加したことを検知しやすくする。

---

# 30. versioned policy data

complimentary token のモデル一覧と上限は OpenAI の program policy であり、将来変更される可能性が高い。

コード内に以下のコメントを残す。

```text
Policy snapshot: 2026-09-06
Source:
https://help.openai.com/en/articles/10306912
```

quota policy を runner logic に埋め込まず、独立した定義へ集約する。

例えば、

```go
type ComplimentaryPolicy struct {
    AsOf       time.Time
    LargeModels map[string]struct{}
    SmallModels map[string]struct{}
}
```

将来更新しやすくする。

MVP では network から policy を自動取得しない。

HTML scraping は禁止。

---

# 31. tests

## 31.1 profile tests

最低限。

- default CX_HOME path
- `CX_HOME` override
- plus path
- api path
- invalid profile
- Windows path handling を platform-neutral な関数で検証

## 31.2 Codex runner tests

fake executable を利用し、

- `CODEX_HOME` が child process にだけ設定される
- arguments が順序を保って透過される
- stdin/stdout/stderr が接続可能
- exit code が透過される
- `cx login plus` の arguments が `login`
- `cx login api` の arguments が `login --with-api-key`

を検証する。

実ユーザーの Codex credential をテストで触らない。

## 31.3 Usage API tests

`httptest.Server` を利用する。

最低限。

- Authorization header
- start time
- end time
- service_tier grouping
- model grouping
- project filter
- pagination
- 401
- 403
- 429
- 500
- timeout
- malformed JSON
- oversized or invalid response

実 OpenAI API を unit test から呼ばない。

## 31.4 quota calculation tests

以下を fixture 化する。

### Tier 2 Large

```text
quota = 250000
usage = 100000
remaining = 150000
```

### Tier 4 Large

```text
quota = 1000000
usage = 100000
remaining = 900000
```

### Tier 2 Small

```text
quota = 2500000
```

### Tier 4 Small

```text
quota = 10000000
```

### quota crossing

```text
complimentary_used = 247000
eligible_traffic = 281000
quota = 250000
estimated_remaining = 0
```

### cached tokens

`input_cached_tokens` を input tokens に追加して二重計上しないこと。

### unknown model

warning を生成し既存 pool に勝手に加算しない。

### unknown service tier

クラッシュせず raw tier を保持する。

### usage tier unknown

usage は表示するが remaining は null。

---

# 32. security requirements

必須。

- Codex `auth.json` を解析しない
- Codex API key を保存しない
- Admin API key を保存しない
- secret を CLI args に要求しない
- secret を stdout に出さない
- secret を stderr に出さない
- secret を debug log に出さない
- Admin key は Authorization header のみに使用
- `OPENAI_ADMIN_KEY` 未設定時は request しない
- regular `OPENAI_API_KEY` に fallback しない

可能であれば test で secret value が output に含まれないことを確認する。

---

# 33. README requirements

README に最低限以下を書く。

## Install

`go install` の例。

release binary の利用方法。

## Initial setup

Plus。

```bash
cx login plus
```

API。

Linux、WSL、macOS の一例。

```bash
printenv OPENAI_API_KEY | cx login api
```

PowerShell の一例。

```powershell
$env:OPENAI_API_KEY | cx login api
```

Codex credential の実際の保存方法は Codex に委譲されることを書く。

## Launch

```bash
cx plus
cx api
```

## Quota

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

Admin API key は Codex 用 API key より強い organization-level permission を持つため、取り扱いに注意する旨を書く。

## Selected projects

```bash
export CX_OPENAI_PROJECT_IDS="proj_aaa,proj_bbb"
cx quota
```

## Limitation

`cx quota` の remaining は OpenAI が返した authoritative remaining balance ではなく、Usage API と公開 quota policy から計算した推定値であることを明記する。

---

# 34. exit codes

基本。

```text
0 success
1 runtime failure
2 invalid CLI usage
```

Codex launch mode では可能な限り Codex child process の exit code をそのまま返す。

`cx quota` では、

- API authentication error
- HTTP error
- parse error
- timeout

は exit code 1。

quota 0 自体はエラーではなく exit code 0。

---

# 35. acceptance criteria

MVP 完了条件。

- `go build ./...` が成功する
- `go test ./...` が成功する
- Windows、Linux、macOS を考慮した path 実装になっている
- `cx plus` で plus profile の `CODEX_HOME` を使って Codex が起動する
- `cx api` で api profile の `CODEX_HOME` を使って Codex が起動する
- 両 profile の認証状態が独立する
- `cx login plus` が Codex login を呼ぶ
- `cx login api` が Codex login `--with-api-key` を呼ぶ
- Codex arguments を透過できる
- `cx status` が両 profile を確認できる
- `cx path` が script-friendly な path を返す
- `cx quota` が Organization Usage API を利用する
- `cx quota` が UTC 00:00 から現在までを集計する
- `cx quota` が Large と Small の 2 pool を分離する
- `cx quota` が input と output token を合算する
- cached token を二重計上しない
- `cx quota` が Usage Tier 1-2 と 3-5 の上限差を扱える
- `cx quota` が quota crossing case を考慮する
- `cx quota --json` が machine-readable output を返す
- Admin API key がログや error output に露出しない
- `auth.json` を直接操作するコードが存在しない
- unit test から実 OpenAI API を呼ばない

---

# 36. 実装優先順位

## Phase 1

profile switching。

- profile path
- `cx plus`
- `cx api`
- arguments passthrough
- process exit code
- `CX_HOME`
- `CX_CODEX_BIN`

## Phase 2

profile management。

- `login`
- `status`
- `path`
- `profiles`
- help

## Phase 3

complimentary token usage。

- Usage API client
- Admin key handling
- UTC window
- pagination
- model pool classification
- service tier grouping
- quota calculation
- human-readable report

## Phase 4

operability。

- `--json`
- `--verbose`
- `--usage-tier`
- `--project`
- environment configuration
- README
- release build

Phase ごとに build と test を通す。

---

# 37. Codex への実装指示

この仕様を実装する際は、まず repository を確認して既存コードがある場合はその構造を尊重すること。

新規 repository の場合は、本仕様の package structure を参考に最小構成から開始する。

実装時の原則。

- KISS
- YAGNI
- secret を扱う独自仕組みを作らない
- Codex credential internals に依存しない
- OpenAI の undocumented API を利用しない
- quota calculation と HTTP access を分離する
- OS 固有コードを必要最小限にする
- testability を優先する
- 不要な framework を追加しない

実装後、最低限以下を実行して結果を報告する。

```bash
go fmt ./...
go vet ./...
go test ./...
go build ./...
```

可能なら以下の cross compile も確認する。

```bash
GOOS=windows GOARCH=amd64 go build ./cmd/cx
GOOS=linux GOARCH=amd64 go build ./cmd/cx
GOOS=darwin GOARCH=arm64 go build ./cmd/cx
```

実 Codex login や実 OpenAI Admin API を CI や自動テストから実行してはならない。

最後に以下を報告する。

- 実装した command
- file structure
- tests
- security considerations
- platform-specific limitations
- 未実装事項
- 実 API で手動確認が必要な項目

---

# 38. OpenAI 公式仕様参照

実装時点で仕様変更がないか確認すること。

OpenAI Usage API:

https://developers.openai.com/api/reference/ruby/resources/admin/subresources/organization/subresources/usage/methods/completions

OpenAI Usage API overview:

https://developers.openai.com/api/reference/python/resources/admin/subresources/organization/subresources/usage

Admin API Keys:

https://platform.openai.com/docs/api-reference/admin-api-keys

Data Sharing と complimentary daily tokens:

https://help.openai.com/en/articles/10306912

重要な現行仕様。

- complimentary token は eligible shared traffic に自動適用される
- Usage Dashboard では data sharing incentive tier として確認できる
- quota は model pool 単位で共有される
- Large pool は Tier 1-2 で 250K、Tier 3-5 で 1M tokens/day
- Small pool は Tier 1-2 で 2.5M、Tier 3-5 で 10M tokens/day
- quota reset は毎日 00:00 UTC
- 1 request が残量を超える場合、その request 全体が通常課金となる
- Usage API は `service_tier` と `model` で group by 可能
- Organization Usage API の利用には Admin API key が必要

