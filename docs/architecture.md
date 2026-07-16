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
3. **単独 Alt のメニュー動作は既定で二段抑制する。** 空打ち候補の Alt down 時に
   未割当 VK `0x07` の down/up を自己注入し、Win32 型のメニュー動作を抑える。さらに
   他キーを伴わない Alt up callback 内で `VK_F24` の down/up を自己注入し、対象アプリから
   入力列を `Alt down → ... → F24 down/up → Alt up` に見せる。assigned VK を使うことで
   Electron/Chromium と Web のキー処理にも chord として認識させる。`Alt+F24` は対象アプリにも
   届くため、定数 `suppressAltMenuFocus` で無効化可能にし、アプリ横断試験を必須とする。
4. **フックと UI を別 OS スレッドに分離する。** フックの生存性をトレイ、描画、同期 API から隔離する。
5. **自己注入だけを識別する。** `LLKHF_INJECTED` だけで除外せず、`dwExtraInfo` の固有マーカーも一致した
   イベントだけを自己注入として無視する。
6. **成功を偽装しない。** IME 送出 API の戻り値を確認し、送出失敗時に通常の A/あ OSD を表示しない。
7. **対象ウィンドウを固定する。** Alt 空打ち成立時の前面 HWND と処理時の前面 HWND が異なる場合は
   切替要求を破棄する。
8. **初期対象は日本語・US 系キーボードレイアウトと Microsoft IME。** AltGr レイアウトと第三者 IME は
   検証対象だが、合格するまで対応済みとは扱わない。
9. **Enter送信ガードだけがキーを消費する。** 対象アプリ（exe 名一致）が前面のとき、Enter 単独を
   Shift+Enter（改行）へ、Ctrl+Enter を素の Enter（送信）へ置換するため、該当する Enter の
   down/up 対だけをブロックする。Alt を含む他のすべての物理キーは従来どおり決してブロックしない。

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
│   ├ Alt down / clean up でメニュー抑制（既定有効）│
│   └ feedGuard(): Enter送信ガード              │
│      （対象アプリのみ Enter を置換・消費）      │
│  EVENT_SYSTEM_FOREGROUND WinEvent            │
│   └ 前面 exe のガード対象判定をキャッシュ       │
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
- `hookProc` は固定量の状態更新、必要時の二段メニュー抑制入力、フック自身への
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
- `KBDLLHOOKSTRUCT.vkCode == VK_MENU` の Alt は `LLKHF_EXTENDED` により
  左=`VK_LMENU` / 右=`VK_RMENU` へ正規化してから状態機械へ渡す。すでに左右別の VK は変更しない。
- `LLKHF_INJECTED` かつ `dwExtraInfo == ownInputTag` の場合だけ自己注入として状態機械から除外する。
- 他プロセス由来の注入イベントは対象アプリにも届く入力なので、進行中の空打ちをキャンセルできる。
- 注入された Alt は空打ち追跡を開始しない。物理 Alt だけをトリガーとする。
- 戻り値は原則 `CallNextHookEx` の戻り値とし、Alt を含む物理キーをブロックしない。
  唯一の例外は Enter送信ガード（§4.5）が置換する Enter イベントで、このときだけ
  `CallNextHookEx` を呼ばず非 0 を返して消費する。

`ownInputTag` は 64-bit 固定値とし、IME 入力とメニュー抑制入力の全 `KEYBDINPUT.dwExtraInfo` に設定する。
これは所有権判定用であり、セキュリティ境界ではない。

### 4.2 Alt メニューフォーカス抑制

- 機能が有効で、他キーが down でない空打ち候補として `tracking` を開始し、かつ
  `suppressAltMenuFocus == true` の場合、物理 Alt の初回 down で未割当 VK `0x07` の down/up を
  `SendInput` する。全イベントに `ownInputTag` を付ける。既押下キーがある chord には送らない。
- `tracking` のまま物理 Alt up を迎えた場合は、IME 切替の保持時間を超えていても、callback 内で
  `VK_F24 (0x87)` の down/up を `SendInput` する。低レベルフックはキーイベントが対象キューへ
  post される前に呼ばれるため、F24 を物理 Alt-up より先に入力ストリームへ挿入する。
- 他キーで `canceled` になった Alt chord には F24 を送らない。実際の他キーがすでに単独 Alt 判定を
  キャンセルするためであり、Alt+Tab 等へ不要な `Alt+F24` を追加しない。
- Alt 自体はブロックしないため、Alt+Tab、Alt+F4、メニューアクセラレータは通常経路へ流す。
- 未割当 VK `0x07` は Electron/Chromium や DOM キーイベントから見えない場合があるため、
  Alt-up 側では assigned key の F24 を併用する。これによりネイティブメニュー、Electron の
  Alt-up 判定、Web アプリの単独 Alt 判定のいずれにも対応する。
- 失敗・一部送出なら抑制不能として記録するが、
  物理 Alt は引き続き素通しする。
- 2件中1件だけ挿入された場合は、割当済み F24 の down が残らないようタグ付き key-up を
  追加送出する。cleanup も失敗した場合は callback 外で診断ログへ記録する。
- `Alt+F24` は対象アプリにも届く。Electron、Edge/Chrome の DOM、JetBrains、ゲーム、RDP、
  キーカスタマイズソフトとの干渉を受け入れ試験する。
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

### 4.5 Enter送信ガード `feedGuard`

対象アプリでの誤送信を防ぐため、Enter の意味を置換する。空打ち機械とは独立した
第二の状態機械（`enterguard.go`、純粋ロジック）として実装し、同じイベント列を
`feedAlt` の後に食わせる。

**発動条件（すべて AND）:**

1. フック全体が有効、かつ Enter送信ガードが有効（トレイでトグル、フックスレッドが所有）。
2. 前面ウィンドウのプロセス exe 名が `enterGuardTargetExes` に一致（キャッシュ判定）。
3. 物理（非注入）の `VK_RETURN` down。他プロセス注入の Enter は素通し。テンキー Enter も対象。
4. 修飾が「なし」または「Ctrl のみ」。Shift / Alt / Win のいずれかが down なら介入しない。

**状態機械（`idle` / `swallow` の 2 状態 + 修飾キー down 集合）:**

| 現状態 | 入力 | 次状態 / 動作 |
|---|---|---|
| idle | 物理 Enter down、発動条件成立、修飾なし | swallow。ブロックし、タグ付き Shift+Enter を注入（改行） |
| idle | 物理 Enter down、発動条件成立、Ctrl のみ | swallow。ブロックし、押下中の Ctrl 一時解放→Enter→Ctrl 復元をタグ付き注入（送信） |
| idle | Enter down（Shift/Alt/Win を含む、非対象、無効、注入） | idle のまま素通し |
| idle | Enter up | idle のまま素通し（down を素通しした押下、resync で孤立した up） |
| swallow | 物理 Enter down（auto-repeat） | swallow のままブロック。再注入しない |
| swallow | 物理 Enter up | idle。ブロック（down/up を対で飲み、対応漏れの up をアプリへ届けない） |
| swallow | 注入 Enter down/up | swallow のまま素通し（第三者注入に干渉しない） |
| 任意 | 修飾キー down/up | 追跡集合の更新のみ。汎用 `VK_SHIFT`/`VK_CONTROL` は左右別コードへ正規化 |

- Ctrl+Enter の置換では、`GetAsyncKeyState` で物理押下中の左右 Ctrl を特定し、押下中の側だけを
  解放・復元する。どちらも up と報告される稀なレースでは Ctrl 解放なしで Enter のみ注入し、
  診断カウンタへ記録する。挿入数不足時はタグ付き key-up/down で best-effort 復旧する。
- 注入はすべて `ownInputTag` 付きの 1 回の `SendInput` で行い、自己注入フィルタにより
  両状態機械から除外されるため再帰しない。
- resync（起動・有効化・ガードトグル・セッション/電源復帰）は修飾集合を `GetAsyncKeyState` から
  再構築して idle へ戻す。swallow 中に resync した押下の孤立 up は idle が無視するので無害。

**空打ち機械との排他:** Enter イベントはブロック有無に関係なく `feedAlt` にも先に食わせ、
Alt 保持中の Enter は従来どおりタップキャンセル要因になる。ガードは Alt down 中の Enter に
介入しないため、両機械が同一イベントで同時に「介入」する状況は構造上発生しない。

**前面対象キャッシュ:** 主経路はフックスレッドに張った `WINEVENT_OUTOFCONTEXT` の
`SetWinEventHook(EVENT_SYSTEM_FOREGROUND)`。コールバックは登録スレッドのメッセージポンプ経由で
配送されるため、キーボード callback の外で `GetWindowThreadProcessId` →
`OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)` → `QueryFullProcessImageNameW` を実行して
`{hwnd, isTarget}` を更新する。キャッシュはフックスレッド専有で同期不要。
Enter down 時に `GetForegroundWindow()` とキャッシュの hwnd が不一致なら（フォーカス変更直後の
配送遅延窓）、callback 内で同期解決へフォールバックする。これは固定量原則（NFR-1）からの
**有界な逸脱**（syscall 3 本、発生は稀）であり、頻度を診断カウンタ `guardSyncResolve` で観測する。
WinEvent フック設置に失敗した場合は毎 Enter down が同期解決になるだけで、機能は維持される。

**IME 変換中の Enter（CON-9）:** 変換確定の Enter もガードが置換してしまう。他プロセスの変換中
状態を確実に検出する公式 API がないため v1 では検出せず、実機挙動を記録する。発動判定は
`feedGuard` の `active` 算出 1 箇所に集約してあり、将来「変換中なら介入しない」条件をここへ
追加できる。

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
- `assets/alt-ime-icon.ico` の 16 / 20 / 24 / 32 / 40 / 48 / 64 / 128 / 256px PNG を
  `RT_ICON`、その選択表を `RT_GROUP_ICON` ID 1 として manifest と同じ
  `rsrc_windows_amd64.syso` に埋め込む。トレイは実行モジュールの ID 1 を読み込み、exe と意匠を統一する。
- アイコン原稿は墨色 `#303236` と白 `#f7f7f2` のキーキャップ形状とし、
  `mkicon.go` → `mkrsrc.go` の順で標準ライブラリだけを使って再生成する。
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

Enter送信ガード（§4.5）はこの作法の実装例である: 独立状態機械（`enterguard.go`）を追加し、
排他仕様（Alt down 中の Enter に介入しない、Enter はタップキャンセル要因のまま）を実装前に
確定した。IME 切替を伴わないため二段配送は使わず、置換注入を callback 内で完結させている。

---

## 9. 制約と対応範囲

- **UIPI:** 標準権限から高権限アプリへの監視・入力注入・ウィンドウメッセージは制約を受ける。
  管理者アプリ内での動作を保証する配布形態は、管理者起動または署名済み UIAccess を別途設計判断する。
- **セキュアデスクトップ:** UAC プロンプト、サインイン画面等は対象外。
- **IME:** Microsoft IME を初期対応対象とする。Google 日本語入力、ATOK 等は検証合格後に対応表へ加える。
- **OS:** Windows 10 1903 の ImeOn/ImeOff 対応更新以降、および Windows 11 の x64 を対象とする。
- **キーボードレイアウト:** 日本語・US 系を初期対象とし、AltGr レイアウトは保証外。
- **メニュー抑制:** 未割当 VK `0x07` は正式なメニュー抑制 API ではなく、`Alt+F24` は対象アプリへ
  届くため、アプリ固有ショートカットとの互換性リスクが残る。
- **OSD:** 実 IME 状態ではなく、切替要求の送出結果を表示する。
- **Enter送信ガード:** IME 変換中の確定 Enter も置換される（CON-9、v1 未対応・実機記録）。
  対象判定は exe 名のみで、ブラウザ内 Web チャットは対象外（CON-10）。UIPI により管理者権限
  アプリの exe 名解決や注入が失敗し得る（解決失敗は非対象として素通し）。

---

## 10. 必須の検証観点

- 状態機械は Win32 から分離した純粋ロジックとして、遷移表の全行をユニットテストする。
- 左右 Alt、既押下修飾、両 Alt、長押し、repeat、外部注入、自己注入、不整合列を表形式でテストする。
- Enter送信ガードも同様に遷移表の全行（修飾組合せ、左右/両 Ctrl、auto-repeat、注入 Enter、
  resync 中断、非対象素通し）と exe 名照合をユニットテストする。
- Win32 バインディングの構造体サイズとフィールドオフセットを windows/amd64 向けテストで固定する。
- 実機では Microsoft IME、主要アプリ、権限差、Explorer 再起動、スリープ復帰、混在 DPI を確認する。
- フック callback の処理時間をデバッグビルドで計測し、最大値を記録する。callback 内でログ I/O はしない。

---

## 11. 参考（一次情報）

- alt-ime-ahk 本家: https://github.com/karakaram/alt-ime-ahk
- Virtual-Key Codes: https://learn.microsoft.com/en-us/windows/win32/inputdev/virtual-key-codes
- ImeOn/ImeOff キー設計: https://learn.microsoft.com/en-us/windows-hardware/design/component-guidelines/keyboard-japan-ime
- SendInput: https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendinput
- WM_SYSKEYUP: https://learn.microsoft.com/en-us/windows/win32/inputdev/wm-syskeyup
- Electron BaseWindow（単独 Alt と自動非表示メニュー）: https://www.electronjs.org/docs/latest/api/base-window
- Outlook キーボードショートカット（KeyTips = Alt）: https://support.microsoft.com/en-us/accessibility/keyboard-shortcuts-for-outlook
- LowLevelKeyboardProc: https://learn.microsoft.com/en-us/windows/win32/winmsg/lowlevelkeyboardproc
- KBDLLHOOKSTRUCT: https://learn.microsoft.com/en-us/windows/win32/api/winuser/ns-winuser-kbdllhookstruct
- SetWindowsHookExW: https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setwindowshookexw
- SendMessageTimeoutW: https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendmessagetimeoutw
- ImmGetDefaultIMEWnd: https://learn.microsoft.com/en-us/windows/win32/api/imm/nf-imm-immgetdefaultimewnd
- Shell_NotifyIconW: https://learn.microsoft.com/en-us/windows/win32/api/shellapi/nf-shellapi-shell_notifyiconw
- Icons: https://learn.microsoft.com/en-us/windows/win32/menurc/icons
- Resource File Formats: https://learn.microsoft.com/en-us/windows/win32/menurc/resource-file-formats
- LoadIconW: https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-loadiconw
- WTSRegisterSessionNotification: https://learn.microsoft.com/en-us/windows/win32/api/wtsapi32/nf-wtsapi32-wtsregistersessionnotification
- WM_POWERBROADCAST: https://learn.microsoft.com/en-us/windows/win32/power/wm-powerbroadcast
- DPI Awareness: https://learn.microsoft.com/en-us/windows/win32/hidpi/setting-the-default-dpi-awareness-for-a-process
