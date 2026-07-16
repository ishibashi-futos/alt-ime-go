## Project Overview

左右 Alt の空打ちで IME を OFF/ON する Windows 常駐ツール（alt-ime-ahk 相当）の
Go スクラッチ実装。外部 Go 依存はゼロで、Win32 API は `internal/win32` の標準
`syscall` 直接バインディング経由でのみ呼ぶ。対象は Windows 10 1903+ / Windows 11
x64、配布物は GUI サブシステムの単一 `alt-ime-go.exe`。

## Commands & Tooling

すべてリポジトリルートで実行する。Go 1.26.2 は mise で管理する:

```sh
mise install
go version   # go1.26.2 と表示されること
```

Codex サンドボックスでは、Go コマンドより先に同じ shell 内で次を設定する:

```sh
export GOCACHE=/tmp/alt-ime-go-cache
export GOMODCACHE=/tmp/alt-ime-go-modcache
mkdir -p "$GOCACHE" "$GOMODCACHE"
```

検査・テスト:

```sh
gofmt -l .                                # 出力が空であること
go test ./...                             # ホストテスト（OS 非依存パッケージ）
GOOS=windows GOARCH=amd64 go vet ./...    # Windows/amd64 静的検査
mkdir -p /tmp/alt-ime-go-testbins && \
  GOOS=windows GOARCH=amd64 go test -c -o /tmp/alt-ime-go-testbins/ ./...
                                          # Windows 向けテストのコンパイル確認
```

Windows 実機ではテストをそのまま実行する:

```sh
go test ./...
```

クロスビルド:

```sh
GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui -s -w" -o alt-ime-go.exe ./cmd/alt-ime-go
```

アイコンと Windows リソース（`cmd/alt-ime-go/rsrc_windows_amd64.syso`）の再生成
（icon → syso の順。syso はコミット対象）:

```sh
go run ./tools/mkicon
go run ./tools/mkrsrc
```

## Rules

- `go.mod` に require を追加しない（外部 Go 依存ゼロ。`golang.org/x/sys` も禁止）。
- `syscall.SyscallN` の呼び出しと Win32 API バインディングは `internal/win32` にだけ置く。
- パッケージ依存方向は `app → {win32, hookstate, config}`、`hookstate → win32` のみ。
  `internal/config` は他の internal パッケージを import しない。循環 import を作らない。
- `internal/hookstate` と `internal/config` に build tag 付きファイルを置かない
  （ホストで `go test ./...` が通る状態を維持する）。
- フック callback から到達するコード（`internal/app/hook.go` の `hookProc` /
  `handleKey` / `feedGuard` / `guardForeground` / `sendSuppressor`）で禁止:
  UI・GDI・Shell・IMM32 の呼び出し、ファイル I/O、`win32.Debugf`、
  `SendMessageTimeout`、無期限待機。診断は atomic カウンタに記録し、
  フックスレッドのメッセージループで排出する。
- 外部 HWND への同期メッセージ送信は `win32.SendMessageTimeout`（有限期限）のみ。
  期限なし SendMessage のバインディングを追加しない。
- 物理 Alt をブロックしない。callback が非 0 を返してよいのは Enter送信ガードが
  置換する Enter の down/up 対だけで、対応する up と auto-repeat も対で飲む。
- `nCode != HC_ACTION` では `lParam` を参照しない。
- 自己注入の判定は `LLKHF_INJECTED` かつ `dwExtraInfo == win32.OwnInputTag` の
  両立時のみ。自己注入入力はすべて `win32.OwnInputTag` を付けて送出する。
- 設定定数の追加先は `internal/config`。OSD 寸法は 96 DPI 基準で定義し
  `config.ScaleDPI` でスケールする。
- 仕様・設定・受け入れ条件の変更時は [docs/requirements.md](docs/requirements.md) と
  [docs/architecture.md](docs/architecture.md) を同時更新する。

## References

設計判断・スレッドモデル・落とし穴と Win32 一次情報リンクは
[docs/architecture.md](docs/architecture.md)、要求 ID（FR/NFR/CON）は
[docs/requirements.md](docs/requirements.md) を参照する。
