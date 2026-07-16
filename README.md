# alt-ime

左右 Alt キーの「空打ち」で IME を OFF/ON する Windows 常駐ツール。
[alt-ime-ahk](https://github.com/karakaram/alt-ime-ahk) 相当の機能を、AutoHotkey ランタイムに
依存せず **Go + 標準 `syscall` のみ**（外部依存ゼロ）でスクラッチ実装したもの。
切替要求の送出結果を macOS 風の OSD で表示し、タスクトレイに常駐する。

> **検証状態:** Windows 実機での動作確認は未実施。検証状態と実機での合否判定項目は
> [docs/todo.md](docs/todo.md) が所有する。

## 機能

| 操作 | 動作 |
|---|---|
| 左 Alt 空打ち | 前面ウィンドウへ IME OFF を要求し、成功時に `A` の OSD を表示 |
| 右 Alt 空打ち | 前面ウィンドウへ IME ON を要求し、成功時に `あ` の OSD を表示 |
| 送出失敗時 | 赤系の `!` OSD（成功表示は偽装しない） |
| Alt+Tab 等の chord | 通常動作（Alt はブロックしない） |
| 単独 Alt | 既定でメニューバーへのフォーカス移動を抑制（本家互換） |
| 対象アプリでの Enter 単独 | Enter送信ガード: Shift+Enter（改行）に置換して誤送信を防止 |
| 対象アプリでの Ctrl+Enter | 素の Enter（送信）に変換。Shift/Alt/Win を含む chord は介入しない |
| トレイアイコン | 有効/無効切替・Enter送信ガード切替・終了。マウスとキーボード両対応、Explorer 再起動後に自動復旧 |
| 多重起動 | 同一セッション内の 2 個目は起動せず通知して終了 |

- 「空打ち」= Alt を押した時点で他キーが押されておらず、保持中にも他キーを押さず、
  500ms 以内に離す操作。
- OSD は「切替要求を入力ストリームへ挿入できた」ことの表示であり、実際の IME 状態を
  確認した表示ではない（[docs/requirements.md](docs/requirements.md) CON-5）。
- Enter送信ガードの対象アプリは前面ウィンドウのプロセス exe 名で判定する
  （既定: M365 Copilot / Claude Desktop）。IME 変換中の Enter（変換確定）の扱いと
  既知の残存リスクは同 CON-9 を参照。

## 動作要件

- Windows 10 1903 以降 / Windows 11（x64）。`VK_IME_ON/OFF` 対応の
  [ImeOn/ImeOff キー設計](https://learn.microsoft.com/en-us/windows-hardware/design/component-guidelines/keyboard-japan-ime)
  に依存するため。
- 初期対応対象は Microsoft IME と日本語・US 系キーボードレイアウト。
  Google 日本語入力・ATOK・AltGr レイアウト・RDP は検証合格まで対応保証外。

## ビルド・開発

ツールチェーンの導入、テスト、静的検査、クロスビルド、アイコン/リソース再生成の
コマンドは [CLAUDE.md](CLAUDE.md) の Commands & Tooling が所有する。
生成物は単一の `alt-ime-go.exe`（GUI サブシステム、PerMonitorV2 DPI manifest と
multi-size アイコンを埋め込み）。

## 設定

設定はすべてコード上の定数（[internal/config/tunables.go](internal/config/tunables.go)）で、
変更後に再ビルドする。各定数の意味・既定値・変更時の注意は同ファイルのコメントが所有する。

## 設計・制約

- 設計とアーキテクチャ（スレッドモデル、二段配送、自己注入識別、メニュー抑制、
  Enter送信ガードの置換規則）: [docs/architecture.md](docs/architecture.md)
- 要求仕様と制約・既知リスク（UIPI、セキュアデスクトップ、`Alt+F24` の互換性、
  変換中推定の限界、SmartScreen 等）: [docs/requirements.md](docs/requirements.md)
- Windows 実機検証チェックリスト: [docs/todo.md](docs/todo.md)
