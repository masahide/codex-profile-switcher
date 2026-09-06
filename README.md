# codex-profile-switcher (`cx`)

`cx` は、Codex CLI の実行環境を `CODEX_HOME` で profile ごとに分離する小さなクロスプラットフォーム CLI です。MVP では `chatgpt` と `api` を提供します。

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

まず Codex CLI が PATH にあることを確認してください。profile ごとの認証は Codex に委譲します。

ChatGPT account authentication:

```bash
cx login chatgpt
```

OpenAI API key authentication:

```bash
printenv OPENAI_API_KEY | cx login api
```

PowerShell:

```powershell
$env:OPENAI_API_KEY | cx login api
```

`codex login --with-api-key` は API key を stdin から読むため、`cx login api` を端末から単独で実行せず、上記のように pipe してください。API key の入力、credential の保存、token refresh は Codex CLI が行います。`cx` は key を引数で受け取らず、Codex の `auth.json` を読み書き・解析しません。

ChatGPT account authentication は Free、Go、Plus、Pro などの ChatGPT プランで利用でき、プランによって利用上限が変わります。認証方式の profile はプラン名ではなく認証方式を表すため、プランを変更しても `cx chatgpt` のまま使えます。

## Launch

```bash
cx chatgpt
cx api
cx api exec "このPRをレビューして"
cx api --model gpt-5.6-sol
```

Codex arguments は順序を変えずに透過します。

profile directory はデフォルトで次の場所です。

```text
<UserHome>/.codex-profiles/chatgpt
<UserHome>/.codex-profiles/api
```

`cx` は `CODEX_HOME` 全体を分離するため、認証だけでなく `config.toml` や Codex のローカル状態も profile ごとに独立します。既存の `<UserHome>/.codex/config.toml` は自動コピー・同期されません。必要な設定は profile ごとの `config.toml` に別途用意してください。

環境変数で変更できます。

```bash
CX_HOME=/mnt/d/codex-profiles cx api
```

Codex executable は通常 `codex` を PATH から探します。別の executable を使う場合は `CX_CODEX_BIN` を指定します。

```bash
CX_CODEX_BIN=/opt/codex/bin/codex cx chatgpt
```

確認用コマンド:

```bash
cx profiles
cx path chatgpt
cx path api
cx status
cx status chatgpt
```

## Quota estimate

`cx quota` は当日の Organization Usage API の completions usage を取得し、公開されている complimentary-token policy snapshot と Usage Tier から推定値を計算します。

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

unknown model は勝手に Large/Small へ分類せず、推定から除外します。`service_tier` の raw value は diagnostics として扱い、未文書化の literal を complimentary usage の判定根拠にはしません。

## Security boundary

- Codex の `auth.json` を直接操作しません。
- `config.toml` と Codex のローカル状態を profile 間で共有しません。
- ChatGPT OAuth token、Codex API key、Admin API key を保存・解析しません。
- Codex 子プロセスへ `OPENAI_ADMIN_KEY`、`OPENAI_API_KEY`、`CODEX_API_KEY`、`CODEX_ACCESS_TOKEN` を継承しません。
- `cx quota` は Admin API key を HTTPS の `Authorization: Bearer` header にだけ設定します。
- Dashboard の private API、HTML scraping、browser automation は使用しません。
- profile directory は新規作成時に Unix で mode `0700` を指定します。

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
- [Admin API keys](https://developers.openai.com/api/reference/python/resources/admin/subresources/organization/subresources/admin_api_keys)
