## プロジェクト概要

alt-ime-ahk 相当（左右 Alt の空打ちで IME を OFF/ON）を **Go でスクラッチ実装する**
Windows 常駐ツール。切替要求の送出結果を macOS 風 OSD で可視化し、タスクトレイに常駐する。

配布バイナリ名は `alt-ime-go.exe`。墨色×白の multi-size icon を exe とトレイで共用する。

## 技術スタック・制約（厳守）

- 言語: **Go**（ローカルは mise で `go@1.26.2` を使用）
- **外部依存ゼロ。** 標準 `syscall` でシステム DLL の Win32 API を直接呼ぶ。
  systray/x-sys 等を追加する場合は、実装前に設計判断を仰ぐ。
- ターゲット: `GOOS=windows GOARCH=amd64`
- GUI サブシステム: `-ldflags "-H windowsgui"`
- 対応 OS: ImeOn/ImeOff 対応更新以降の Windows 10 1903+ / Windows 11 x64
- DPI: manifest の `PerMonitorV2` を既定とする。

## 予定するビルド / 検査

```sh
GOOS=windows GOARCH=amd64 go test ./...
GOOS=windows GOARCH=amd64 go vet ./...
gofmt -l .
GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui -s -w" -o alt-ime-go.exe .
```

> GUI 挙動、IME 実効性、フック、トレイ、DPI は Windows 実機でしか合否判定できない。
> クロスビルド成功を動作確認済みと表現しない。

## ディレクトリ構成

```
.
├─ main.go                  # UI スレッド: ライフサイクル、切替要求の再検証、終了処理
├─ hook.go                  # フック専用スレッドと WH_KEYBOARD_LL callback
├─ ime.go                   # IME 切替送出（VK / IMM32）
├─ osd.go                   # レイヤード OSD ウィンドウ
├─ tray.go                  # タスクトレイ
├─ win32.go                 # Win32 バインディング（syscall 直接呼び出し）
├─ win32types.go            # Win32 定数・構造体（OS 非依存・レイアウトテスト対象）
├─ tapstate.go              # 空打ち状態機械（純粋ロジック・OS 非依存）
├─ enterguard.go            # Enter送信ガード状態機械（純粋ロジック・OS 非依存）
├─ tunables.go              # 設定定数・OSD 寸法・DPI スケール（OS 非依存）
├─ *_test.go                # 状態機械、Tunables、構造体レイアウト（ホストでも実行可）
├─ alt-ime.manifest         # PerMonitorV2 DPI manifest
├─ assets/                  # SVG/PNG 原稿と multi-size ICO
├─ mkicon.go                # アイコン生成器（標準ライブラリのみ、go:build ignore）
├─ mkrsrc.go                # manifest と icon を .syso 化する生成器（go:build ignore）
├─ rsrc_windows_amd64.syso  # 生成済みリソースオブジェクト（コミット対象）
├─ go.mod                   # 外部依存なし
├─ README.md
└─ docs/
   ├─ architecture.md
   ├─ requirements.md
   └─ todo.md
```

## 確定アーキテクチャ（詳細は docs/architecture.md）

- **2 OS スレッド:** UI スレッドとフック専用スレッドをそれぞれ `runtime.LockOSThread()` する。
- **フック層:** `WH_KEYBOARD_LL`。状態判定と固定量の処理だけを行い、切替は UI へ `PostMessage` する。
- **有効状態:** フック専用スレッドが所有し、無効中は状態追跡とメニュー抑制を行わない。
- **空打ち状態機械:** `idle / tracking / canceled`。Alt down 前から保持中のキーもキャンセル条件に含める。
- **自己注入:** `LLKHF_INJECTED` だけで判定せず、`dwExtraInfo == ownInputTag` も一致した入力だけを除外する。
- **メニュー抑制:** 既定では空打ち候補の Alt down 時にタグ付き未割当 VK `0x07`、他キーを伴わない Alt up 時にタグ付き `VK_F24` を送る。
- **二段配送:** Alt-up callback 復帰後に UI へ切替要求を送り、IME 送出直前に Alt 解放を確認する。
- **対象競合防止:** 前面 HWND が変わった要求、または Alt が未解放の要求は破棄する。
- **Enter送信ガード:** 対象アプリ（前面プロセスの exe 名が `enterGuardTargetExes` に一致）でのみ、
  Enter 単独／Ctrl+Enter の down/up 対を callback で消費し、置換の選択と注入は UI スレッドへ
  二段配送（`msgHookDispatchGuard`→`msgGuardEnter`、UI で前面再検証）。Ctrl+Enter、または
  変換中ヒューリスティック成立かつ「`IMC_GETOPENSTATUS` が open または応答なし」なら素の
  Enter（送信 / 変換確定。Ctrl は一時解放→復元）、明確に closed のときだけタグ付き
  Shift+Enter（改行）。IME 問い合わせは `GetGUIThreadInfo` で実フォーカスウィンドウを
  解決してから行う（WebView2 はトップレベルと IME スレッドが別）。ガード注入キーには
  実スキャンコードを付与する（Chromium の DOM `code` 対策）。
  Shift/Alt/Win を含む chord と注入 Enter には介入しない。前面判定は
  `EVENT_SYSTEM_FOREGROUND` WinEvent のキャッシュ + Enter down 時の同期解決フォールバック
  （`guardSyncResolve` で計測）。トレイで個別トグル（`msgHookSetEnterGuard`）。
- **IME:** 主経路は `VK_IME_ON/OFF` の絶対指定。`SendInput` の送出数を必ず検査する。
- **IMM32:** 明示選択時だけ使用し、通常の `SendMessage` ではなく 100ms の `SendMessageTimeout` を使う。
- **OSD:** 送出成功時だけ A/あ、送出失敗時は `!`。実 IME 状態を確認済みとは扱わない。
- **トレイ:** `NIM_SETVERSION`、キーボード操作、`TaskbarCreated` 再登録、多重起動防止に対応する。

## 重要な落とし穴

- `nCode < 0` / 非 `HC_ACTION` では `lParam` を参照しない。
- フックは原則 `CallNextHookEx` の戻り値を返す。Alt は決してブロックしない。ブロックしてよいのは
  Enter送信ガードが置換する Enter の down/up 対だけで、対応する up と auto-repeat も対で飲む。
- 他プロセスの IME 変換中状態を確実に検出する公式 API はない。ガードはキー列からの変換中
  ヒューリスティック（文字キーで疑い開始、Enter/Esc/IME切替キー/フォーカス変更でクリア）+
  UI スレッドでの `IMC_GETOPENSTATUS` で推定し、変換中と推定した Enter は素の Enter を
  再注入して確定を維持する。誤推定は fail-open（素の Enter = アプリ既定の送信）に倒す設計で、
  マウスクリック確定直後の Enter は送信になり得る。クリア条件を安易に増やさない
  （stale-false は変換確定を壊す）。
- 外部ツールの注入入力を一律無視しない。進行中の空打ちのキャンセル要因になり得る。
- `LowLevelHooksTimeout` 超過時、Windows 7+ ではフックが通知なしに削除され得る。UI 処理をフックスレッドへ持ち込まない。
- `SendInput` とウィンドウメッセージは UIPI の制約を受ける。送出成功と実 IME 状態を混同しない。
- 同期的な外部 HWND 呼出しを無期限に待たない。
- Alt メニュー抑制の VK `0x07` は正式 API ではなく、`VK_F24` はアプリにも届く。Electron、ブラウザ、JetBrains、RDP、ゲーム等の互換試験を必須とする。
- OSD の古いタイマが新しい表示を消さないよう、世代または期限を検証する。
- Explorer 再起動後にトレイを再登録する。
- 無効化、セッション/電源復帰時にフック状態機械をリセットする。
- 終了前に `UnhookWindowsHookEx` し、フック停止確認後に UI リソースを逆順解放する。

## 拡張の作法

- トリガー追加は独立状態機械として実装し、成立時に既存の二段配送へ
  `open`、`triggerVK`、`targetHWND` を渡す。
- Alt と新トリガーの優先順位・排他を仕様化してから実装する。
- OSD 値は 96 DPI 基準の Tunables にまとめ、対象 DPI へスケールする。
- 設定変更時は `docs/requirements.md`、`docs/architecture.md`、`docs/todo.md` を同時更新する。

## 参考（一次情報）

- alt-ime-ahk 本家: https://github.com/karakaram/alt-ime-ahk
- ImeOn/ImeOff キー設計: https://learn.microsoft.com/en-us/windows-hardware/design/component-guidelines/keyboard-japan-ime
- SendInput: https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendinput
- LowLevelKeyboardProc: https://learn.microsoft.com/en-us/windows/win32/winmsg/lowlevelkeyboardproc
- KBDLLHOOKSTRUCT: https://learn.microsoft.com/en-us/windows/win32/api/winuser/ns-winuser-kbdllhookstruct
- SetWindowsHookExW: https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setwindowshookexw
- SendMessageTimeoutW: https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendmessagetimeoutw
- Shell_NotifyIconW: https://learn.microsoft.com/en-us/windows/win32/api/shellapi/nf-shellapi-shell_notifyiconw
- DPI Awareness: https://learn.microsoft.com/en-us/windows/win32/hidpi/setting-the-default-dpi-awareness-for-a-process
