# alt-ime

左右 Alt キーの「空打ち」で IME を OFF/ON する Windows 常駐ツール。
[alt-ime-ahk](https://github.com/karakaram/alt-ime-ahk) 相当の機能を、AutoHotkey ランタイムに
依存せず **Go + 標準 `syscall` のみ**（外部依存ゼロ）でスクラッチ実装したもの。
切替要求の送出結果を macOS 風の OSD で表示し、タスクトレイに常駐する。

> **検証状態:** コード実装済み・静的検査（gofmt / go vet / ユニットテスト / クロスビルド /
> PE 検証（GUI サブシステム・DPI manifest・multi-size icon）済み。
> **Windows 実機での動作確認は未実施**。実機での合否判定項目は
> [docs/todo.md](docs/todo.md) の「Windows 実機のリリース判定」を参照。

## 機能

| 操作 | 動作 |
|---|---|
| 左 Alt 空打ち | 前面ウィンドウへ IME OFF を要求し、成功時に `A` の OSD を表示 |
| 右 Alt 空打ち | 前面ウィンドウへ IME ON を要求し、成功時に `あ` の OSD を表示 |
| 送出失敗時 | 赤系の `!` OSD（成功表示は偽装しない） |
| Alt+Tab 等の chord | 通常動作（Alt はブロックしない） |
| 単独 Alt | 既定でメニューバーへのフォーカス移動を抑制（本家互換） |
| 対象アプリでの Enter 単独 | Enter送信ガード: ブロックして Shift+Enter（改行）に置換。誤送信を防止 |
| 対象アプリでの Ctrl+Enter | 素の Enter（送信）に変換。Shift/Alt/Win を含む chord は介入しない |
| トレイアイコン | 有効/無効切替・Enter送信ガード切替・終了。マウスとキーボード両対応、Explorer 再起動後に自動復旧 |
| 多重起動 | 同一セッション内の 2 個目は起動せず通知して終了 |

- 「空打ち」= Alt を押した時点で他キーが押されておらず、保持中にも他キーを押さず、
  500ms 以内に離す操作。
- OSD は「切替要求を入力ストリームへ挿入できた」ことの表示であり、
  **実際の IME 状態を確認した表示ではない**（TSF を含む実状態検証は行わない）。
- Enter送信ガードの対象アプリは前面ウィンドウのプロセス exe 名で判定する
  （既定: M365 Copilot / Claude Desktop。`tunables.go` の `enterGuardTargetExes`）。
- IME 変換中の Enter（変換確定）は、キー入力列からの変換中推定と IME 状態の問い合わせ
  （`IMC_GETOPENSTATUS`）により素の Enter として届け、**変換確定はそのまま動く**。
  他プロセスの変換中状態を確実に検出する公式 API はないため推定であり、外れた場合は
  素の Enter を届ける方向（アプリ既定の動作）に倒す。既知の残存リスク:
  マウスクリックで変換確定した直後の Enter は改行ではなく送信になり得る。

## 動作要件

- Windows 10 1903 以降 / Windows 11（x64）。`VK_IME_ON/OFF` 対応の
  [ImeOn/ImeOff キー設計](https://learn.microsoft.com/en-us/windows-hardware/design/component-guidelines/keyboard-japan-ime)
  に依存するため。
- 初期対応対象は Microsoft IME と日本語・US 系キーボードレイアウト。
  Google 日本語入力・ATOK・AltGr レイアウト・RDP は検証合格まで対応保証外。

## ビルド

Go 1.26 系（リポジトリの `mise.toml` は `go@1.26.2` を指定）。

```sh
gofmt -l .
GOOS=windows GOARCH=amd64 go vet ./...
GOOS=windows GOARCH=amd64 go test ./...   # テストバイナリの実行は Windows 上でのみ可能
GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui -s -w" -o alt-ime-go.exe .
```

- 生成物は単一の `alt-ime-go.exe`（GUI サブシステム、コンソールなし）。
- DPI manifest（Per-Monitor V2）と墨色×白の multi-size アプリアイコンは
  `rsrc_windows_amd64.syso` 経由で埋め込む。アイコンを変更する場合は
  `go run mkicon.go`、続けて `go run mkrsrc.go` で syso を再生成する
  （いずれの生成器も標準ライブラリのみ）。

### テストについて

ユニットテスト（空打ち状態機械・OSD スケーリング・Win32 構造体レイアウト）は OS 非依存に
書かれており、開発ホスト上の `go test ./...` でも実行できる。
`GOOS=windows GOARCH=amd64 go test ./...` は **Windows 以外のホストではテストバイナリを
実行できず失敗する**（クロスコンパイルは成功する）。Windows 実機では同コマンドが
そのまま実行できる。

## 設定

設定は現状すべてコード上の定数（`tunables.go`）。変更後は再ビルドする。

| 定数 | 既定値 | 意味 |
|---|---|---|
| `tapMaxHoldMs` | 500 | 空打ちと判定する最長保持時間 (ms) |
| `suppressAltMenuFocus` | true | 単独 Alt のメニュー抑制（Alt-down で VK `0x07`、clean Alt-up で `VK_F24` を注入） |
| `imeControl` | `imeControlVK` | IME 制御方式。`imeControlIMM32` で IMM32 経路に切替（自動フォールバックなし） |
| `imm32TimeoutMs` | 100 | IMM32 経路の `SendMessageTimeoutW` 期限 (ms) |
| `enterGuardTargetExes` | `m365copilot.exe`, `claude.exe` | Enter送信ガードの対象 exe 名（basename、小文字） |
| `enterGuardDefaultEnabled` | true | 起動時の Enter送信ガード有効状態（トレイで切替可能） |
| `osdBase` ほか | — | OSD の寸法（96 DPI 基準）・色・表示/フェード時間 |

## アーキテクチャ概要

詳細は [docs/architecture.md](docs/architecture.md)。

- **2 OS スレッド:** UI スレッドとフック専用スレッドを `runtime.LockOSThread()` で分離。
  `WH_KEYBOARD_LL` callback は状態機械更新と固定量の処理のみを行い、UI/GDI/ログ I/O を
  持ち込まない（`LowLevelHooksTimeout` 対策）。
- **二段配送:** Alt-up callback から直接 IME を送らず、フック自身のキューへ Post →
  callback 復帰後に UI へ `PostMessage` → UI 側で前面 HWND・Alt 解放を再検証してから
  `SendInput`。前面が変わった要求や Alt 未解放（50ms 再確認後）の要求は破棄する。
- **自己注入識別:** `LLKHF_INJECTED` かつ `dwExtraInfo == ownInputTag` の入力だけを
  自己注入として無視。他プロセスの注入入力は進行中の空打ちをキャンセルする。
- **状態機械:** `idle / tracking / canceled` の純粋ロジック（`tapstate.go`）。
  遷移表全行・境界値・時刻 wraparound・再同期をユニットテストで固定。
- **解放順序:** 終了時はフック停止を確認してから、トレイ → タイマ → OSD/GDI →
  ウィンドウ → mutex の逆順で解放する。

## 既知の制約

- **UIPI:** 標準権限から管理者権限アプリへの入力注入・メッセージ送信は制限され、
  `SendInput` の戻り値だけでは原因を特定できない。失敗時は `!` OSD と
  `OutputDebugStringW`（[DebugView](https://learn.microsoft.com/sysinternals/downloads/debugview)
  等で閲覧）に記録される。
- **セキュアデスクトップ**（UAC プロンプト、サインイン画面）は対象外。
- **メニュー抑制**は Alt-down で本家由来の未割当 VK `0x07`、他キーを伴わない Alt-up で
  Electron/Chromium や Web アプリにも届く `VK_F24` を注入する。このため対象アプリへ
  `Alt+F24` の keydown/keyup が届き、DOM のキー監視からも見える。アプリ固有の割り当て、
  JetBrains IDE・RDP・ゲーム等で
  干渉する場合は `suppressAltMenuFocus = false` で
  無効化できる。無効化すると空打ち検出はそのまま動くが、単独 Alt で Windows 既定の
  メニューバーフォーカス移動が復活する。
- **OSD は送出結果**であり、IME が要求を受理したかは表示しない（`VK_IME_ON/OFF` を
  尊重しない IME が存在する）。
- 署名なし exe のため SmartScreen / Defender の警告対象になり得る。

## Windows 実機での確認手順（未検証項目）

以下は本リポジトリの開発環境（macOS クロスビルド）では検証できないため未検証。
Windows 10/11 x64 実機で確認する。

1. `GOOS=windows GOARCH=amd64 go test ./...`（実機ではそのまま実行可能）。
2. `alt-ime-go.exe` を起動し、メモ帳等で左 Alt 空打ち → 半角英数 + `A` OSD、
   右 Alt 空打ち → ひらがな + `あ` OSD。
3. Alt+Tab / Alt+F4 / Alt+Space / Alt+英字が通常動作し、誤切替しないこと。
4. 単独 Alt でメニューバーにフォーカスが移らず、VS Code のカスタムメニューと
   Outlook on the Web の KeyTips が開かないこと。
5. Shift/Ctrl/Win 併用・両 Alt・長押し（>500ms）・Alt repeat で切り替わらないこと。
6. トレイ: 右クリック/Enter でメニュー、有効/無効切替、終了。
   `taskkill /f /im explorer.exe && start explorer` 後のアイコン復旧。
7. 2 個目の起動が「既に起動しています」で終了すること。
8. 100%/150%/200% と混在 DPI で OSD の位置・サイズ・文字が正しいこと。
9. スリープ復帰・ロック解除直後の最初の入力で誤切替しないこと。
10. Enter送信ガード: 対象アプリ（M365 Copilot / Claude Desktop）で Enter → 改行のみ、
    Ctrl+Enter → 送信、Shift+Enter は従来どおり。非対象アプリでは一切介入しないこと。
    IME 変換中の Enter で変換確定でき、確定後 2 回目の Enter が改行になること。
    推定が外れるケース（マウスクリック確定直後の Enter 等）の挙動を記録すること。
11. 終了後にフック・トレイアイコン・プロセスが残らないこと。

全チェックリストは [docs/todo.md](docs/todo.md) を参照。

## ドキュメント

- [docs/requirements.md](docs/requirements.md) — 要求仕様・受け入れ基準
- [docs/architecture.md](docs/architecture.md) — 設計とアーキテクチャ
- [docs/todo.md](docs/todo.md) — 進捗と実機リリース判定チェックリスト
