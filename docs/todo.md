# ToDo — alt-ime

> 凡例: [ ] 未着手 / [~] 進行中 / [x] 完了
>
> 現状: **実装済み・静的検査済み。** `gofmt` / `go vet`（windows/amd64）/ ユニットテスト
> （ホストネイティブ実行）/ クロスビルド / PE 検証
> （GUI サブシステム・DPI manifest・multi-size icon）を
> 通過。**Windows GUI 実挙動・IME 実効性・フック・トレイ・DPI は実機未検証**（下段の
> リリース判定リストが未消化）。

---

## フェーズ 0: 実装基盤

- [x] `go.mod`、`main.go`、`win32.go`、`README.md` を作成
- [x] Win32 型・定数・構造体・API 戻り値規約を `win32.go` / `win32types.go` に集約
- [x] `PerMonitorV2` manifest と multi-size アイコンを exe に埋め込むビルド方法を確定
      （`alt-ime.manifest` + `assets/alt-ime-icon.ico` + `mkrsrc.go` 生成の
      `rsrc_windows_amd64.syso`。PE 解析で埋め込み確認済み）
- [x] 名前付き mutex によるユーザーセッション単位の多重起動防止
- [x] UI スレッドとフック専用 OS スレッドの起動・ready・停止 handshake
- [x] 部分初期化失敗時の逆順 cleanup
- [x] `GetMessage == -1` を含む致命エラーの MessageBox と `OutputDebugStringW`

## フェーズ 1: フック・空打ち状態機械

- [x] Win32 非依存の状態機械（`tapstate.go`）を `idle / tracking / canceled` で実装
- [x] 現在 down 中の非自己入力キー集合を管理（固定長配列・callback 内アロケーションなし）
- [x] 起動・有効化・セッション/電源復帰時に callback 外で down 中キー集合を再同期
- [x] 既押下修飾、両 Alt、repeat、長押し、不整合イベント、時刻 wraparound のテスト
- [x] 物理 Alt だけを開始条件とし、外部注入キーはキャンセル要因として扱う
- [x] `ownInputTag` を全自己注入へ設定し、タグ一致イベントだけを除外
- [x] `nCode < 0` / 非 `HC_ACTION` の即時 pass-through
- [x] callback からフック自身へ dispatch を Post し、復帰後に UI へ二段配送
- [x] `open`、`triggerAltVK`、`targetHWND` を整数値で渡し、Go ポインタを渡さない
- [x] `PostThreadMessage` / `PostMessage` 失敗を処理
      （callback 内は atomic カウンタで検出しループ側で記録、ループ側は即時記録）
- [x] `msgHookSetEnabled` でフック側の有効状態を更新し、無効中は追跡・メニュー抑制を停止
- [x] 無効化・再有効化・状態期限切れ時の reset

## フェーズ 2: Alt メニューフォーカス抑制

- [x] `suppressAltMenuFocus` を既定 true で追加
- [x] 他キーなしで tracking を開始する物理 Alt down 時に、タグ付き未割当 VK `0x07` down/up を固定 2 入力で送出
- [x] 他キーを伴わない物理 Alt up 時に、タグ付き `VK_F24` down/up を固定 2 入力で送出
- [x] 抑制キーが1件だけ挿入された場合は、key-up を追加送出して stuck key を予防
- [x] 送出数を検査し、失敗しても物理 Alt はブロックしない
- [ ] 単独 Alt でメニューフォーカスが移らないことを確認（実機）
- [ ] VS Code でカスタムメニューへフォーカスが移らないことを確認（実機）
- [ ] Outlook on the Web で KeyTips が表示されないことを確認（実機）
- [ ] Alt+Tab / Alt+F4 / Alt+Space / Alt+英字への干渉がないことを確認（実機）
- [ ] Edge/Chrome の DOM、JetBrains、RDP、ゲーム、Razer Synapse 等で `Alt+F24` の副作用がないことを記録（実機）
- [x] 問題環境で抑制を無効化した場合の挙動を README に記載

## フェーズ 3: IME 制御と対象整合性

- [x] 空打ち成立時の `targetHWND` を取得し、UI 処理時に再検証
- [x] UI 側でもトリガー Alt の解放を確認し、必要時だけ最大 50ms の再確認タイマ
- [x] VK 方式: `VK_IME_ON/OFF` down/up を 1 回の `SendInput` で送出
- [x] `SendInput` 戻り値 2 を `switchInserted`、2 未満を `switchFailed` として扱う
- [x] IMM32 方式: `SendMessageTimeoutW`、`SMTO_ABORTIFHUNG | SMTO_BLOCK`、100ms
- [x] VK→IMM32 の自動フォールバックを行わず、方式を明示選択
- [x] 失敗時の API 名、戻り値、`GetLastError` を `OutputDebugStringW` へ記録
- [ ] Microsoft IME 新旧での実効性を Windows 実機確認

## フェーズ 4: OSD

- [x] 通常 OSD: OFF=`A` / ON=`あ`、送出成功時のみ表示
- [x] 失敗 OSD: 赤系 `!`、同一失敗の連続通知を抑制（1500ms、成功で解除）
- [x] `WS_EX_NOACTIVATE`、クリック透過、最前面、タスクバー非表示
- [x] `SW_SHOWNOACTIVATE` / `SWP_NOACTIVATE` を使用
- [x] 96 DPI 基準値から対象モニタ DPI へ全寸法をスケール
- [x] `rcWork` 内への配置、`WM_DPICHANGED` でフォント・リージョン再生成
- [x] フェードタイマに世代を持たせ、古い `WM_TIMER` を無効化
- [x] GDI オブジェクトとリージョンの所有権・破棄順を実装
      （`SetWindowRgn` 成功時はシステム所有、失敗時のみ削除。リーク有無は実機確認項目）

## フェーズ 5: トレイ・終了処理

- [x] `NIM_ADD` 後に毎回 `NIM_SETVERSION(NOTIFYICON_VERSION_4)`
- [x] `WM_CONTEXTMENU` / `NIN_SELECT` / `NIN_KEYSELECT` を処理
- [x] メニュー終了後に `NIM_SETFOCUS`
- [x] `TaskbarCreated` 受信後にトレイを再登録（NIM_ADD 失敗時は NIM_MODIFY）
- [x] 有効/無効切替時にフック状態機械を reset
- [x] WTS セッション通知と電源復帰通知でフック状態機械を reset
- [x] フック停止確認後にトレイ、タイマ、GDI、ウィンドウ、mutex を解放
      （フック無応答時は 2000ms のフォールバックタイマで続行）
- [x] 初期トレイ登録失敗時は MessageBox 後に安全終了
- [x] 墨色×白の独自 multi-size アイコンを exe に埋め込み、トレイでも共用

## フェーズ 6: 静的検査・ビルド

- [x] 状態機械ユニットテスト
- [x] windows/amd64 の Win32 構造体サイズ・フィールドオフセットテスト
- [x] `gofmt -l .` が空
- [x] `GOOS=windows GOARCH=amd64 go vet ./...` が成功
- [~] `GOOS=windows GOARCH=amd64 go test ./...` が成功
      （クロスコンパイル成功・同一テストのホストネイティブ実行は全緑。
      テストバイナリの実行は Windows 実機でのみ可能なため、実機での実行は未実施）
- [x] `GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui -s -w" -o alt-ime-go.exe .` が成功
- [x] exe に GUI subsystem、DPI manifest、multi-size icon が埋め込まれていることを確認（`debug/pe` 解析）

## フェーズ 7: Enter送信ガード

- [x] Win32 非依存の状態機械（`enterguard.go`）を `idle / swallow` + 修飾キー追跡で実装
- [x] 遷移表全行・左右/両 Ctrl・auto-repeat・注入 Enter・resync のユニットテスト
- [x] `hookProc` にブロック経路を新設（ガードが置換する Enter だけ `CallNextHookEx` を呼ばず非 0 を返す）
- [x] タグ付き Shift+Enter 注入（改行）と Ctrl 一時解放→Enter→Ctrl 復元注入（送信）、挿入数検査と best-effort 復旧
- [x] `EVENT_SYSTEM_FOREGROUND` WinEvent による前面 exe のガード対象キャッシュ
      （フックスレッド専有、Enter down 時の同期解決フォールバックは `guardSyncResolve` で計測）
- [x] `GetWindowThreadProcessId` / `OpenProcess` / `QueryFullProcessImageNameW` バインディングと exe 名照合（`matchGuardTarget`）
- [x] トレイメニュー「Enter送信ガード」トグル（`msgHookSetEnterGuard`、既定 ON）
- [x] 有効化・ガードトグル・セッション/電源復帰時に両状態機械を resync しキャッシュ再解決
- [x] 置換選択を UI スレッドへ二段配送化（`msgHookDispatchGuard`→`msgGuardEnter`、
      UI 側で前面再検証。callback は消費と配送のみ）
- [x] IME 変換確定 Enter の緩和（CON-9/FR-24）: キー列からの変換中ヒューリスティック +
      `IMC_GETOPENSTATUS`（有界）で推定し、変換中と推定した Enter は素の Enter を再注入
- [x] 変換中ヒューリスティックのユニットテスト（開始キー・終了キー・編集キー・注入キー・resync）
- [x] ~~IME 変換中の確定 Enter の挙動を記録（v1 既知課題）~~ →
      **実機で確定不能を確認**（M365 Copilot、2026-07-16）。上記の緩和を実装
- [x] 緩和後も確定不能との実機報告（同日）を受けた追加修正:
      (1) IME 問い合わせ先を `GetGUIThreadInfo` の実フォーカスウィンドウへ変更
      （WebView2 はトップレベルと IME スレッドが別で、変換中でも closed が返っていた疑い）、
      (2) 応答が得られない場合は素の Enter へ fail-open、
      (3) ガード注入キーへ実スキャンコード付与（Chromium の DOM `code` 対策）、
      (4) `guardTrace` で置換判定の内訳を DebugView へ記録（既定 ON、検証完了後に false へ）
- [ ] 対象アプリ（M365 Copilot / Claude Desktop）で Enter→改行、Ctrl+Enter→送信を確認（実機）
- [ ] Shift+Enter / Alt+Enter / Win+Enter が従来どおり動作することを確認（実機）
- [ ] 非対象アプリ（メモ帳 / VS Code / ブラウザ）で一切介入しないことを確認（実機）
- [ ] IME 変換中の Enter で確定できること（かな→Enter、候補選択→Enter）を確認（実機・FR-24）
- [ ] IME OFF での Enter が改行になり、確定後 2 回目の Enter も改行になることを確認（実機）
- [ ] ヒューリスティックが外れるケース（マウスクリック確定直後の Enter 等）の挙動を記録（実機・CON-9）
- [ ] Enter 長押しで stuck key・意図しない送信がないことを確認（実機）
- [ ] Ctrl+Enter 後に Ctrl が論理押しっぱなしにならないことを確認（続く Ctrl+英字）（実機）
- [ ] Alt+Tab 直後の即 Enter で対象判定が正しいこと、`guardSyncResolve` の頻度を DebugView で記録（実機）
- [ ] トレイトグル境界（Enter 押下中のトグル含む）とロック/スリープ復帰直後の Enter を確認（実機）
- [ ] Alt 空打ち・メニュー抑制への回帰がないこと、`measureHookLatency` の最大値が悪化しないことを確認（実機）

---

## Windows 実機のリリース判定

### 基本動作

- [ ] 左 Alt 空打ち → IME OFF、右 Alt 空打ち → IME ON
- [ ] Shift/Ctrl/Win を先に保持した Alt で誤切替しない
- [ ] Alt 長押し、両 Alt、repeat、不整合イベントで誤切替しない
- [ ] 単独 Alt のメニューフォーカスを抑制する
- [ ] Alt chord がすべて通常動作する
- [ ] 通常時の空打ち成立→送出開始 p95 が 50ms 以下

### IME / 入力

- [ ] Microsoft IME（新）
- [ ] Microsoft IME（以前のバージョン）
- [ ] VK 方式と IMM32 方式を個別に記録
- [ ] Google 日本語入力（合格まで対応保証外）
- [ ] ATOK（合格まで対応保証外）
- [ ] AltGr レイアウトで少なくとも誤切替・文字入力破壊がない（機能保証外）

### アプリ横断

- [ ] メモ帳
- [ ] Word・Excel
- [ ] Edge・Chrome
- [ ] VS Code
- [ ] Outlook on the Web
- [ ] Windows Terminal・PowerShell・コマンドプロンプト
- [ ] UWP / Windows App SDK 系アプリ
- [ ] JetBrains IDE
- [ ] リモートデスクトップ / 仮想マシン
- [ ] キーカスタマイズソフト併用

### トレイ・ライフサイクル

- [ ] `GOOS=windows GOARCH=amd64 go test ./...` を実機で実行して全緑
- [ ] トレイをマウスとキーボードで操作
- [ ] Explorer 再起動後にアイコン復旧
- [ ] 二重起動を拒否
- [ ] 有効/無効の切替境界で誤発火しない
- [ ] 終了後にフック・アイコン・プロセスが残らない
- [ ] exe とトレイに同じ独自アイコンが表示され、主要 DPI で潰れ・にじみがない
- [ ] スリープ復帰・ロック解除後にフックが生存し、最初の入力で誤発火しない

### DPI / 表示

- [ ] 100% / 150% / 200%
- [ ] 異なる DPI の複数モニタを往復
- [ ] 「あ」が豆腐にならない
- [ ] OSD がフォーカス・クリックを奪わない
- [ ] 連続切替しても新しい OSD が早期消去されない

### 権限・セキュリティ

- [ ] 標準権限アプリ内
- [ ] 管理者権限アプリ内での検出・送出・失敗表示を個別に記録
- [ ] UAC セキュアデスクトップでは動作対象外であることを確認
- [ ] SmartScreen / Defender の結果を記録

---

## 後続フェーズ

- [ ] 設定ファイル対応
- [ ] ログオン自動起動
- [ ] コード署名
- [ ] UIAccess + 署名 + 安全なインストール形態の設計判断
- [ ] Ctrl+Shift 方式（割り当て・優先順位・排他の仕様確定後）
- [ ] Enter送信ガード: 変換中推定の精度向上（実機記録の結果、必要なら。候補ウィンドウ検出や
      `EVENT_OBJECT_IME_SHOW/HIDE` WinEvent の併用等）
- [ ] Enter送信ガード: auto-repeat での改行連打対応の判断（v1 は repeat を飲むだけ）
