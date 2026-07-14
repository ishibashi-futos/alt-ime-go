# 設計とアーキテクチャ — alt-ime (Go / スクラッチ実装)

本書は alt-ime-ahk 相当ツールを Go でスクラッチ実装するための確定設計をまとめる。
対象は Alt 方式（左右 Alt の空打ちで IME を OFF/ON）。Ctrl+Shift 方式は拡張余地として
境界だけを定義し、本フェーズでは実装しない。

> 実装は完了し静的検査（gofmt / go vet / ユニットテスト / クロスビルド / PE 検証)を
> 通過済み。ただし Windows 実機検証は未実施であり、検証前の挙動を
> 「対応済み」「安定動作」と表現しない。

---

## 1. 確定した設計判断

1. **IME 状態は絶対指定する。** 左 Alt は常に OFF、右 Alt は常に ON とし、トグルを使わない。
2. **Alt chord はブロックしない。** フックは常に `CallNextHookEx` へ渡し、Alt+Tab 等を維持する。
3. **単独 Alt のメニューフォーカスは既定で抑制する。** 本家互換のため Alt down 時に
   未割当 VK `0x07` の down/up を自己注入する。これは Microsoft が保証する正式機能ではなく、
   本家実装に基づく互換策なので、定数 `suppressAltMenuFocus` で無効化可能にし、アプリ横断試験を必須とする。
4. **フックと UI を別 OS スレッドに分離する。** フックの生存性をトレイ、描画、同期 API から隔離する。
5. **自己注入だけを識別する。** `LLKHF_INJECTED` だけで除外せず、`dwExtraInfo` の固有マーカーも一致した
   イベントだけを自己注入として無視する。
6. **成功を偽装しない。** IME 送出 API の戻り値を確認し、送出失敗時に通常の A/あ OSD を表示しない。
7. **対象ウィンドウを固定する。** Alt 空打ち成立時の前面 HWND と処理時の前面 HWND が異なる場合は
   切替要求を破棄する。
8. **初期対象は日本語・US 系キーボードレイアウトと Microsoft IME。** AltGr レイアウトと第三者 IME は
   検証対象だが、合格するまで対応済みとは扱わない。

---

## 2. 全体構成

```
  物理キー入力
      │
      ▼
┌─────────────────────────────────────────────┐
│ フック専用 OS スレッド                       │
│  WH_KEYBOARD_LL → hookProc                  │
│   ├ HC_ACTION のみ処理                       │
│   ├ 自己注入マーカーを識別                    │
│   ├ feedAlt(): 空打ち状態機械                 │
│   └ Alt down 時のみメニュー抑制入力（既定有効）│
└──────────────────┬──────────────────────────┘
                   │ PostThreadMessage（callback 復帰）
                   │ → PostMessage(msgSwitch, open+altVK, targetHWND)
                   │ → UI で Alt 解放確認
                   ▼
┌─────────────────────────────────────────────┐
│ UI OS スレッド                               │
│  コントローラウィンドウ                       │
│   ├ 対象 HWND の再検証                        │
│   ├ setIME(open) → 結果判定                   │
│   ├ osdShow / osdShowError                    │
│   └ トレイ・ライフサイクル                     │
│  OSD ウィンドウ                               │
└─────────────────────────────────────────────┘
```

IME、OSD、トレイは UI スレッドだけが所有する。空打ち状態はフック専用スレッドだけが所有する。
共有可変状態を直接読み書きせず、連携は Win32 メッセージで行う。

---

## 3. スレッドモデルと起動順序

### 3.1 UI スレッド

- `main` 冒頭で `runtime.LockOSThread()` する。
- HWND を作る前に DPI Awareness を設定する。原則は manifest の `PerMonitorV2`。
- 多重起動防止用の名前付き mutex を取得する。既存インスタンスがあれば通知して終了する。
- コントローラウィンドウ、OSD ウィンドウ、トレイを生成する。
- フック goroutine を起動し、フック設置の成否を起動時チャネルで 1 回だけ受け取る。
- 成功後に単一の `GetMessage` ループへ入る。`GetMessage == -1` は異常終了として扱う。

### 3.2 フック専用スレッド

- 専用 goroutine で `runtime.LockOSThread()` する。
- `PeekMessage` でスレッドメッセージキューを確実に生成してからフックを設置する。
- `SetWindowsHookExW(WH_KEYBOARD_LL, ...)` と最小の `GetMessage` ループだけを担当する。
- 機能の有効状態もフック専用スレッドが所有し、UI から `msgHookSetEnabled` で更新する。
- 起動、有効化、セッション/電源復帰時は callback の外で `GetAsyncKeyState` により
  down 中キー集合を再同期してから追跡を再開する。
- `hookProc` は固定量の状態更新、必要時のメニュー抑制用 2 入力、フック自身への
  `PostThreadMessage` だけを行う。UI への切替要求は callback 復帰後にフックループから送る。
- UI/GDI/Shell/IMM32、ログ I/O、待機処理は実行しない。
- `LowLevelHooksTimeout` 超過時は Windows 7 以降でフックが通知なしに削除され得るため、
  「一時的に外れる」と仮定しない。専用スレッド化と処理時間計測で予防する。

### 3.3 終了順序

1. UI がトレイ操作を無効化し、フック専用スレッドへ `msgHookStop` を送る。
2. フック側が `UnhookWindowsHookEx` し、UI へ `msgHookStopped` を返して終了する。
3. UI がトレイを `NIM_DELETE`、タイマ停止、GDI オブジェクト破棄、ウィンドウ破棄の順で解放する。
4. mutex を解放し、最後に `PostQuitMessage` する。

初期化途中の失敗も、成功済みリソースをこの逆順で解放する。

---

## 4. キーボードイベント契約

### 4.1 `hookProc`

- `nCode < 0` または `nCode != HC_ACTION` では `lParam` を参照せず、直ちに `CallNextHookEx` する。
- 機能が無効なら空打ち追跡もメニュー抑制も行わず、直ちに `CallNextHookEx` する。
- `wParam` は `WM_KEYDOWN/UP` と `WM_SYSKEYDOWN/UP` のみを受け付ける。
- down/up はメッセージ種別と `LLKHF_UP` が矛盾しないことを前提とし、矛盾時は状態をキャンセルする。
- `LLKHF_INJECTED` かつ `dwExtraInfo == ownInputTag` の場合だけ自己注入として状態機械から除外する。
- 他プロセス由来の注入イベントは対象アプリにも届く入力なので、進行中の空打ちをキャンセルできる。
- 注入された Alt は空打ち追跡を開始しない。物理 Alt だけをトリガーとする。
- 戻り値は常に `CallNextHookEx` の戻り値とし、物理キーをブロックしない。

`ownInputTag` は 64-bit 固定値とし、IME 入力とメニュー抑制入力の全 `KEYBDINPUT.dwExtraInfo` に設定する。
これは所有権判定用であり、セキュリティ境界ではない。

### 4.2 Alt メニューフォーカス抑制

- 機能が有効で、他キーが down でない空打ち候補として `tracking` を開始し、かつ
  `suppressAltMenuFocus == true` の場合、物理 Alt の初回 down で `VK 0x07` の down/up を
  `SendInput` する。全イベントに `ownInputTag` を付ける。既押下キーがある chord には送らない。
- Alt 自体はブロックしないため、Alt+Tab、Alt+F4、メニューアクセラレータは通常経路へ流す。
- `0x07` は未割当 VK を利用する互換策であり、失敗・一部送出なら抑制不能として記録するが、
  物理 Alt は引き続き素通しする。
- JetBrains、ゲーム、RDP、キーカスタマイズソフトとの干渉を受け入れ試験する。
- 問題がある環境向けに定数で無効化できるようにする。設定ファイル対応は後続フェーズ。

### 4.3 空打ち状態機械 `feedAlt`

状態は `idle`、`tracking`、`canceled` の 3 状態とし、追跡対象 VK、押下時刻、
現在 down 中の非自己入力キー集合をフック専用スレッド内に保持する。down 中キー集合は
有効中のイベントで増減し、起動・再有効化・セッション/電源復帰時に callback 外で再同期する。

| 現状態 | 入力 | 次状態 / 動作 |
|---|---|---|
| idle | 物理 LAlt/RAlt down、他キー down なし | tracking。対象 Alt と時刻を保存 |
| idle | 物理 LAlt/RAlt down、他キーが既に down | canceled。対象 Alt up まで切替禁止 |
| tracking | 同じ Alt の repeat down | tracking のまま無視 |
| tracking | 反対 Alt または Alt 以外の down | canceled |
| tracking | 対象 Alt up、保持時間内 | 空打ち成立。前面 HWND を取得し、フック自身へ dispatch を Post |
| tracking | 対象 Alt up、保持時間超過 | idle。切替なし |
| canceled | 対象 Alt up | idle。切替なし |
| 任意 | 不整合な down/up、状態期限切れ | 状態を安全側へリセット。切替なし |

- `tapMaxHoldMs` は既定 500ms とし、0（無期限）は採用しない。
- 時刻は `KBDLLHOOKSTRUCT.time` の 32-bit wraparound を unsigned 差分で扱う。
- 無効化、フック再設置、セッション/電源復帰、不整合検出時に状態をリセットする。
- AltGr は LCtrl と RAlt の合成として現れる場合があり、初期フェーズでは対応保証外とする。
  AltGr レイアウトで誤爆しないことを検証し、対応する場合は独立した設計判断を行う。

### 4.4 切替要求と対象競合

低レベルフックは非同期キー状態が更新される前に呼ばれるため、Alt-up callback から UI へ直接
切替要求を送らない。空打ち成立時に `GetForegroundWindow` で `targetHWND` を取得し、
`open`、`triggerAltVK`、`targetHWND` を `msgHookDispatchSwitch` としてフック自身のキューへ Post する。

callback 復帰後、フックループが
`PostMessage(ctrlHwnd, msgSwitch, open+triggerAltVK, targetHWND)` を送る。値は `WPARAM/LPARAM` に
収まる整数として渡し、Go ポインタを渡さない。Alt 解放の最終確認は、IME を送出する UI 側が行う。

UI 側では以下をすべて満たす場合だけ処理する。

1. 機能が有効。
2. `targetHWND != NULL` かつ `IsWindow(targetHWND)`。
3. `GetForegroundWindow() == targetHWND`。
4. `GetAsyncKeyState(triggerAltVK)` が up。

不一致なら要求を破棄し、IME も OSD も操作しない。これにより、キュー滞留中に別アプリへ
フォーカスが移った場合や Alt がまだ down の場合の誤切替を防ぐ。Alt がまだ down の場合だけ、
UI の one-shot タイマで最大 50ms まで再確認する。対象 HWND が変わるか期限を超えたら破棄する。
アイドル時のポーリングは行わない。各 `PostThreadMessage` / `PostMessage` の失敗も検出する。

---

## 5. IME 制御層

### 5.1 主経路: VK 方式

`SendInput` で `VK_IME_ON(0x16)` または `VK_IME_OFF(0x1A)` の down/up を 2 件まとめて送る。
各入力に `ownInputTag` を設定する。

`setIME(open)` は結果を返す。

- `switchInserted`: 戻り値が 2。入力ストリームへの挿入に成功。
- `switchFailed`: 戻り値が 2 未満。通常の A/あ OSD は表示しない。

`switchInserted` は IME が要求を受理したことまでは保証しない。通常 OSD は
「切替要求を入力ストリームへ正常に挿入した」ことを示すフィードバックと定義する。
実 IME 状態を確認済みであるとは表示しない。

送出失敗時は赤系の `!` OSD を短時間表示し、同じ失敗の連続通知を抑制する。
GUI サブシステムでも診断できるよう `OutputDebugStringW` に API 名と `GetLastError` を出す。
UIPI による `SendInput` 失敗は戻り値や `GetLastError` だけでは原因を特定できない点を明記する。

### 5.2 副経路: IMM32 方式

`ImmGetDefaultIMEWnd(targetHWND)` →
`SendMessageTimeoutW(WM_IME_CONTROL, IMC_SETOPENSTATUS, open)` を使う。

- 通常の `SendMessage` は使用しない。
- `SMTO_ABORTIFHUNG | SMTO_BLOCK`、期限 100ms を既定とする。
- NULL HWND、タイムアウト、UIPI 拒否、API 失敗を `switchFailed` とする。
- VK 送出後に自動で IMM32 を重ねない。VK が挿入されたが IME に無視されたことは同期的に判定できず、
  二重適用は別の副作用を生むため、制御方式は設定で明示的に選ぶ。
- `IMC_GETOPENSTATUS` は TSF・第三者 IMEを含む普遍的な検証手段とはみなさない。

---

## 6. OSD 層

- `WS_POPUP` と `WS_EX_LAYERED | WS_EX_TRANSPARENT | WS_EX_TOOLWINDOW |
  WS_EX_TOPMOST | WS_EX_NOACTIVATE` を使う。
- 表示は `SW_SHOWNOACTIVATE`、位置変更は `SWP_NOACTIVATE` とし、フォーカスを奪わない。
- 通常表示は OFF=`A`、ON=`あ`。失敗表示は `!` とし、色を変える。
- 通常表示は `switchInserted` の場合だけ行う。
- `SetLayeredWindowAttributes(LWA_ALPHA)` と WM_PAINT の GDI 描画を使う。
- 角丸リージョン、フォント、ブラシは所有権を明記し、置換・終了時にリークなく破棄する。

### 6.1 DPI と配置

- manifest で `PerMonitorV2` を宣言する。manifest を使えない開発ビルドだけ、最初の HWND 作成前に
  `SetProcessDpiAwarenessContext` を使用する。
- 幅、高さ、角丸半径、余白、フォントサイズを 96 DPI 基準値から対象 DPI へスケールする。
- 対象は切替要求の `targetHWND` が属するモニタ。`MONITORINFO.rcWork` 内に配置する。
- OSD が異なる DPI のモニタへ移る場合は `WM_DPICHANGED` と推奨矩形を処理し、フォントとリージョンを再生成する。
- フェード開始時は既存タイマを置換し、世代番号または表示期限を検証して古い `WM_TIMER` が
  新しい OSD を消さないようにする。

---

## 7. トレイと常駐ライフサイクル

- `Shell_NotifyIconW(NIM_ADD)` 成功後、毎回 `NIM_SETVERSION(NOTIFYICON_VERSION_4)` を呼ぶ。
- コールバックは `WM_CONTEXTMENU`、`NIN_SELECT`、`NIN_KEYSELECT` を処理し、マウス右クリックだけに依存しない。
- メニュー終了後は `NIM_SETFOCUS` で通知領域へフォーカスを返す。
- `TaskbarCreated = RegisterWindowMessageW("TaskbarCreated")` を保持し、Explorer 再起動後に
  `NIM_ADD` と `NIM_SETVERSION` を再実行する。
- メニューは「有効」「終了」。有効/無効切替時は `msgHookSetEnabled` を送り、フック側は
  状態機械を reset してから新しい有効状態を適用する。UI 側も queued request を再検査する。
- `WTSRegisterSessionNotification` と `WM_POWERBROADCAST` を使い、ロック/解除・電源復帰時に
  フック状態機械へ reset を送る。
- トレイ API の失敗は黙殺せず、初期登録失敗なら MessageBox を表示して起動を中止する。
- 多重起動は名前付き mutex で防止する。mutex 名はユーザーセッション単位とし、Global 名前空間を使わない。

`NOTIFYICONDATAW.cbSize` は Windows 10/11 を対象に現行構造体の `unsafe.Sizeof` を使う。
構造体レイアウトのユニットテストと Windows 実機試験で検証し、失敗時だけ既知バージョンサイズを調査する。

---

## 8. 拡張点: Ctrl+Shift 方式

トリガーは次のインターフェース境界に合わせる。

1. フックイベントを受ける独立状態機械。
2. 成立時に `open`、`triggerAltVK`、`targetHWND` を含む既存の二段配送へ渡す。
3. IME、OSD、トレイ層は変更しない。

ただし Alt と Ctrl+Shift が同じ入力列で同時成立しないよう、トリガー間の優先順位と排他制御は
実装前に仕様化する。Windows 標準の入力言語切替との衝突も受け入れ試験する。

---

## 9. 制約と対応範囲

- **UIPI:** 標準権限から高権限アプリへの監視・入力注入・ウィンドウメッセージは制約を受ける。
  管理者アプリ内での動作を保証する配布形態は、管理者起動または署名済み UIAccess を別途設計判断する。
- **セキュアデスクトップ:** UAC プロンプト、サインイン画面等は対象外。
- **IME:** Microsoft IME を初期対応対象とする。Google 日本語入力、ATOK 等は検証合格後に対応表へ加える。
- **OS:** Windows 10 1903 の ImeOn/ImeOff 対応更新以降、および Windows 11 の x64 を対象とする。
- **キーボードレイアウト:** 日本語・US 系を初期対象とし、AltGr レイアウトは保証外。
- **メニュー抑制:** 未割当 VK `0x07` は正式契約ではないため、アプリ互換性リスクが残る。
- **OSD:** 実 IME 状態ではなく、切替要求の送出結果を表示する。

---

## 10. 必須の検証観点

- 状態機械は Win32 から分離した純粋ロジックとして、遷移表の全行をユニットテストする。
- 左右 Alt、既押下修飾、両 Alt、長押し、repeat、外部注入、自己注入、不整合列を表形式でテストする。
- Win32 バインディングの構造体サイズとフィールドオフセットを windows/amd64 向けテストで固定する。
- 実機では Microsoft IME、主要アプリ、権限差、Explorer 再起動、スリープ復帰、混在 DPI を確認する。
- フック callback の処理時間をデバッグビルドで計測し、最大値を記録する。callback 内でログ I/O はしない。

---

## 11. 参考（一次情報）

- alt-ime-ahk 本家: https://github.com/karakaram/alt-ime-ahk
- Virtual-Key Codes: https://learn.microsoft.com/en-us/windows/win32/inputdev/virtual-key-codes
- ImeOn/ImeOff キー設計: https://learn.microsoft.com/en-us/windows-hardware/design/component-guidelines/keyboard-japan-ime
- SendInput: https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendinput
- LowLevelKeyboardProc: https://learn.microsoft.com/en-us/windows/win32/winmsg/lowlevelkeyboardproc
- KBDLLHOOKSTRUCT: https://learn.microsoft.com/en-us/windows/win32/api/winuser/ns-winuser-kbdllhookstruct
- SetWindowsHookExW: https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setwindowshookexw
- SendMessageTimeoutW: https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendmessagetimeoutw
- ImmGetDefaultIMEWnd: https://learn.microsoft.com/en-us/windows/win32/api/imm/nf-imm-immgetdefaultimewnd
- Shell_NotifyIconW: https://learn.microsoft.com/en-us/windows/win32/api/shellapi/nf-shellapi-shell_notifyiconw
- WTSRegisterSessionNotification: https://learn.microsoft.com/en-us/windows/win32/api/wtsapi32/nf-wtsapi32-wtsregistersessionnotification
- WM_POWERBROADCAST: https://learn.microsoft.com/en-us/windows/win32/power/wm-powerbroadcast
- DPI Awareness: https://learn.microsoft.com/en-us/windows/win32/hidpi/setting-the-default-dpi-awareness-for-a-process
