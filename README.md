# codex-profile-switcher (`cx`)

`cx` は、Codex の設定、セッション、履歴、skills などを現在の環境のまま維持しながら、ChatGPT 認証と OpenAI API キー認証を切り替える小さなクロスプラットフォーム CLI です。

## Install

リリース済みの最新バイナリを取得するワンライナーを用意しています。インストーラーは実行環境に対応するアーカイブを選び、`checksums.txt` で検証してからユーザー領域へインストールします。root／Administrator 権限は要求しません。

macOS、Linux、WSL:

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/codex-profile-switcher/main/install.sh | bash
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/masahide/codex-profile-switcher/main/install.ps1 | iex
```

Windows CMD:

```cmd
curl -fsSL https://raw.githubusercontent.com/masahide/codex-profile-switcher/main/install.cmd -o install.cmd && install.cmd && del install.cmd
```

Unix 系ではデフォルトで `~/.local/bin/cx`、Windows では `%LOCALAPPDATA%\cx\cx.exe` にインストールします。PATH にない場合は、Unix 系ではシェルの起動ファイル、Windows ではユーザー PATH に追加します。PATH の反映には新しいターミナルを開いてください。

バージョンを固定する場合は、次のように指定できます。

```bash
curl -fsSL https://raw.githubusercontent.com/masahide/codex-profile-switcher/main/install.sh | bash -s -- v0.1.0
```

PowerShell／CMD では `CX_VERSION` 環境変数を設定します。

```powershell
$env:CX_VERSION = 'v0.1.0'; irm https://raw.githubusercontent.com/masahide/codex-profile-switcher/main/install.ps1 | iex
```

インストール先やPATH更新を変更する場合は、`CX_INSTALL_DIR`、`CX_NO_PATH_UPDATE=1` を利用できます。

Go が利用できる環境では、次のコマンドでもインストールできます。

```bash
go install github.com/masahide/codex-profile-switcher/cmd/cx@latest
```

ソースからのビルド:

```bash
go build -o cx ./cmd/cx
```

リリース用のクロスコンパイル例:

```bash
GOOS=windows GOARCH=amd64 go build -o cx.exe ./cmd/cx
GOOS=windows GOARCH=arm64 go build -o cx.exe ./cmd/cx
GOOS=linux GOARCH=amd64 go build -o cx ./cmd/cx
GOOS=linux GOARCH=arm64 go build -o cx ./cmd/cx
GOOS=darwin GOARCH=amd64 go build -o cx ./cmd/cx
GOOS=darwin GOARCH=arm64 go build -o cx ./cmd/cx
```

WSL は Linux として扱います。

## Initial setup

まず Codex CLI が PATH にあることを確認してください。認証と credential の保存は Codex に委譲します。

ChatGPT account authentication:

```bash
cx chatgpt
```

OpenAI API key authentication:

Linux、WSL、macOS:

```bash
export CX_OPENAI_API_KEY="sk-..."
cx api
```

PowerShell:

```powershell
$env:CX_OPENAI_API_KEY = "sk-..."
cx api
```

`cx api` は現在の認証状態を確認し、必要な場合だけ `codex logout` の後に `codex login --with-api-key` を実行します。API key は command line argument、環境変数、temporary file ではなく stdin から Codex へ渡します。API key の保存と token refresh は Codex CLI が行い、`cx` は Codex の `auth.json` を読み書き・解析しません。

ChatGPT account authentication は Free、Go、Plus、Pro などの ChatGPT プランで利用でき、プランによって利用上限が変わります。`chatgpt` はプラン名ではなく認証方式を表すため、プランを変更しても `cx chatgpt` のまま使えます。

## Launch

```bash
cx chatgpt
cx api
cx api exec "このPRをレビューして"
cx api --model gpt-5.6-sol
```

Codex arguments は順序を変えずに透過します。

`cx chatgpt` と `cx api` は同じ `CODEX_HOME` を利用します。`CODEX_HOME` を親環境で設定している場合はその値を尊重し、未設定なら Codex の既定値（通常は `~/.codex`）が使われます。`config.toml`、model 設定、MCP、sessions、history、skills などは認証方式間で共有されます。`CX_HOME` や `.codex-profiles` は使用しません。

Codex executable は通常 `codex` を PATH から探します。別の executable を使う場合は `CX_CODEX_BIN` を指定します。

```bash
CX_CODEX_BIN=/opt/codex/bin/codex cx chatgpt
```

確認用コマンド:

```bash
cx status
```

`cx login chatgpt` または `cx login api` は、認証を切り替えるだけで Codex 本体を起動しない形式として利用できます。通常は `cx chatgpt` または `cx api` を使ってください。

認証切替は現在の `CODEX_HOME` の Codex credential cache を変更します。同じ `CODEX_HOME` を使う IDE extension など、他の Codex client にも新しい認証方式が反映される可能性があります。別の Codex session が同じ環境を使用中の間は認証を切り替えないでください。

## Quota estimate

`cx quota` は当日の Organization Usage API の completions usage を取得し、raw `service_tier` が正確に `incentivized-tier` の token を complimentary used として集計します。モデルを Large/Small group に対応づけ、Usage Tier の quota から estimated remaining を計算します。あわせて Organization Costs API から当日の実課金額を補助情報として表示します。

```bash
export OPENAI_ADMIN_KEY="<your-admin-api-key>"
export CX_OPENAI_USAGE_TIER=2
cx quota
cx quota --verbose
cx quota --json
```

PowerShell:

```powershell
$env:OPENAI_ADMIN_KEY = "<your-admin-api-key>"
$env:CX_OPENAI_USAGE_TIER = "2"
cx quota
```

Admin API key は `OPENAI_ADMIN_KEY` からそのリクエストの間だけ読み取ります。保存せず、query parameter、標準出力、標準エラー、debug log、command argument には出しません。`OPENAI_API_KEY` には fallback しません。

Usage Tier は次の優先順位です。

1. `--usage-tier`
2. `CX_OPENAI_USAGE_TIER`
3. 既定値 `1`

対応する値は `1` から `5` です。Tier 1–2 と Tier 3–5 はそれぞれ同じ quota を共有します。

## Selected projects

Data Sharing が selected projects のみで有効な場合は、対象 project IDs を指定してください。

```bash
cx quota --project proj_aaa --project proj_bbb
```

または:

```bash
export CX_OPENAI_PROJECT_IDS="proj_aaa,proj_bbb"
cx quota
```

`--project` が環境変数より優先されます。Data Sharing が selected projects のみで有効なのに scope を指定しない場合、推定値が正確でない可能性があります。

## Important limitation

`cx quota` does not read an official complimentary-token balance. It estimates remaining quota from the Organization Usage API and OpenAI's published complimentary-token policy.

For authoritative verification of complimentary token usage, use the OpenAI Usage Dashboard and group Chat Completions by Service tier. Dashboard では input tokens と output tokens を有効にし、`data sharing incentive tier` を確認してください。

公式 Dashboard:

<https://platform.openai.com/usage/chat-completions>

推定値は Dashboard と一致しない場合があります。Data Sharing の設定、project scope、Usage API の集計遅延、policy や model naming の変更、quota をまたぐ request の扱いによって差が生じます。quota を超える request は一部だけでなく request 全体が通常課金になる可能性があるため、`estimated remaining` は次の request が無料になる保証ではありません。

`default` を含む `incentivized-tier` 以外の traffic は無料使用量に加算しません。unknown model は Large/Small へ分類せず、incentivized-tier であっても推定から除外します。model × service_tier の内訳は `cx quota --verbose` で確認できます。
静的な model mapping は pool の選択にだけ使い、無料判定は Usage API の `service_tier` を優先します。`gpt-6-astra` は 2026-09-06 の実観測に基づく暫定的な Large group mapping です。Help Center の model mapping が更新されるまで、Dashboard を最終確認元にしてください。
公開 policy snapshot または暫定 observed mapping に含まれない model は `Not covered by current complimentary-token policy` として通常表示しますが、quota pool には加算しません。対象 model の追加直後など、policy snapshot が現状に追いついていない可能性を確認しやすくするための表示です。

Costs API の値は当日 00:00 UTC 以降の実課金額です。complimentary tokens は Costs には課金として現れないため、token の `complimentary used` / `estimated remaining` と billed cost は別の値です。Costs API は無料判定には利用しません。Costs API が利用できない場合も、quota estimate は warning とともに表示します。

## Security boundary

- Codex の `auth.json` を直接操作しません。
- `CODEX_HOME` を変更せず、`config.toml` と Codex のローカル状態を認証方式間で共有します。
- ChatGPT OAuth token、Codex API key、Admin API key を `cx` が保存・解析しません。
- API login の key は `CX_OPENAI_API_KEY` から読み取り、Codex の stdin にだけ渡します。Codex 子プロセスへ `OPENAI_ADMIN_KEY`、`OPENAI_API_KEY`、`CX_OPENAI_API_KEY`、`CODEX_API_KEY`、`CODEX_ACCESS_TOKEN` を継承しません。
- `cx quota` は Usage API と Costs API の両方に同じ Admin API key を HTTPS の `Authorization: Bearer` header でだけ設定します。
- Dashboard の private API、HTML scraping、browser automation は使用しません。
- `logout` 後の login に失敗しても、`cx` は以前の credential を保持していないため自動復元を試みません。

## Development

依存関係は Go 標準ライブラリだけです。

```bash
go fmt ./...
go vet ./...
go test ./...
go build ./...
```

テストは fake executable と `httptest` を使い、実 Codex login や実 OpenAI API を呼びません。

## Releases

`v` で始まるタグを push、またはそのタグの GitHub Release を publish すると、GitHub Actions がテストと vet を実行し、次の 6 ターゲットをビルドします。

- Windows amd64 / arm64
- Linux amd64 / arm64
- macOS amd64 / arm64

Windows は zip、Linux/macOS は tar.gz として公開され、`checksums.txt` も添付されます。対象タグの GitHub Release が存在すれば asset を更新し、存在しなければ Release を自動作成します。

```bash
git tag v0.1.0
git push origin v0.1.0
```

## References

- [Data Sharing and complimentary daily tokens](https://help.openai.com/en/articles/10306912)
- [Organization Usage API: completions](https://developers.openai.com/api/reference/ruby/resources/admin/subresources/organization/subresources/usage/methods/completions)
- [Organization Costs API](https://developers.openai.com/api/reference/python/resources/admin/subresources/organization/subresources/usage)
- [Admin API keys](https://developers.openai.com/api/reference/python/resources/admin/subresources/organization/subresources/admin_api_keys)
