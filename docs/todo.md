# Windows 実機検証チェックリスト — alt-ime

未検証項目と検証状態の所有元はこのファイルだけとし、他文書からはここへリンクする。
要求 ID（FR/NFR/CON）は [requirements.md](requirements.md)、設計は
[architecture.md](architecture.md)、コマンドは [CLAUDE.md](../CLAUDE.md) を参照。

> **現在の検証状態:** 実装済み・静的検査済み（`gofmt` / `go vet`（windows/amd64）/
> ホストユニットテスト / Windows 向けテストバイナリのクロスコンパイル /
> クロスビルド / PE 検証（GUI サブシステム・PerMonitorV2 manifest・multi-size icon））。
> **Windows 実機での動作実績はない。** 以下が消化されるまで、未検証の挙動を
> 「対応済み」「動作確認済み」と記載しない。

## テスト実行

- [ ] `go test ./...` を Windows 実機で実行して全緑（テストバイナリは実機でのみ実行できる）

## 入力と状態機械

- [ ] 左 Alt 空打ち → 半角英数 + `A` OSD、右 Alt 空打ち → ひらがな + `あ` OSD
- [ ] Shift/Ctrl/Win を先に保持した Alt 空打ちで切り替わらない
- [ ] 起動直後・再有効化直後にすでに他キーが down でも誤判定しない
- [ ] Alt+Tab / Alt+F4 / Alt+Space / Alt+英字が通常動作し、誤切替しない
- [ ] 両 Alt、Alt repeat、500ms 超の長押し、不整合な down/up で誤切替しない
- [ ] 自己注入は状態機械から除外され、他ツールの注入キーは進行中の空打ちをキャンセルする

## メニュー抑制

- [ ] 単独 Alt でメニューバーへフォーカスが移らない（本家互換）
- [ ] VS Code のカスタムメニューへフォーカスが移らない
- [ ] Outlook on the Web で KeyTips が表示されない
- [ ] Edge/Chrome の DOM、JetBrains IDE、RDP、ゲーム、キーカスタマイズソフト併用で
      `Alt+F24` の副作用を記録する（CON-3）

## IME と対象整合性

- [ ] Microsoft IME（新・以前のバージョン）での実効性を、VK 方式と IMM32 方式それぞれで記録
- [ ] 空打ち直後に前面アプリが変わった要求は破棄され、別アプリを誤切替しない
- [ ] Alt-up callback 復帰前に IME キーを送らず、Alt+ImeOn/ImeOff として誤送出しない
- [ ] `SendInput` が 2 未満を返した場合に A/あ OSD を出さず `!` を表示する
- [ ] IMM32 経路は 100ms 以内に成功またはタイムアウトし、UI とフックを停止させない
- [ ] 通常時の空打ち成立→送出開始の p95 が 50ms 以下（NFR-12）
- [ ] Google 日本語入力・ATOK・AltGr レイアウトを記録し、未合格なら README の対応保証外の記載を維持

## OSD / DPI

- [ ] OSD がフォーカスとクリックを奪わず、指定時間後に消える
- [ ] 100% / 150% / 200% と混在 DPI で位置・サイズ・文字が正しく、「あ」が豆腐にならない
- [ ] 異なる DPI の複数モニタを往復しても正しい
- [ ] 連続切替時に古いタイマが新しい OSD を早期に消さない

## トレイ・ライフサイクル

- [ ] トレイをマウスとキーボードで操作でき、有効/無効・Enter送信ガード・終了が機能する
- [ ] `taskkill /f /im explorer.exe && start explorer` 後にアイコンが復旧する
- [ ] 2 個目の起動が「既に起動しています」で終了する
- [ ] 有効/無効の切替境界で誤発火しない
- [ ] スリープ復帰・ロック解除後にフックが生存し、最初の入力で誤発火しない
- [ ] 終了後にフック・トレイアイコン・タイマ・GDI オブジェクト・プロセスが残らない
- [ ] exe とトレイに同じアイコンが表示され、主要 DPI で潰れ・にじみがない

## Enter送信ガード

- [ ] 対象アプリ（M365 Copilot / Claude Desktop）で Enter 単独が改行になり、送信されない
- [ ] 対象アプリで Ctrl+Enter が送信になる
- [ ] Shift+Enter / Alt+Enter / Win+Enter は従来どおり動作する
- [ ] 非対象アプリ（メモ帳、VS Code、ブラウザ等）では一切介入しない
- [ ] IME 変換中の Enter で変換確定できる（かな入力→Enter、変換候補選択→Enter）（FR-24）
- [ ] IME OFF（直接入力）での Enter は改行になる
- [ ] 変換確定後 2 回目の Enter が改行になる
- [ ] ヒューリスティックが外れるケース（マウスクリック確定直後の Enter 等）の挙動を記録する（CON-9）
- [ ] Enter 長押しで stuck key・意図しない送信が発生しない
- [ ] Ctrl+Enter 後に Ctrl が論理押しっぱなしにならない（続く Ctrl+英字が誤発火しない）
- [ ] Alt+Tab 直後の即 Enter でも対象判定が正しく、`guardSyncResolve` の頻度を DebugView で記録する
- [ ] トレイトグル境界（Enter 押下中のトグルを含む）とロック/スリープ復帰直後の Enter を確認する
- [ ] Alt 空打ち・メニュー抑制への回帰がない

## アプリ横断

- [ ] メモ帳 / Word・Excel / Edge・Chrome / VS Code / Outlook on the Web /
      Windows Terminal・PowerShell・コマンドプロンプト / UWP 系 / JetBrains IDE /
      RDP・仮想マシン / キーカスタマイズソフト併用で必須項目が合格する

## 権限・セキュリティ

- [ ] 標準権限アプリ内で動作する
- [ ] 管理者権限アプリでの検出・送出・失敗表示（`!` OSD と OutputDebugString）を個別に記録する（CON-1）
- [ ] UAC セキュアデスクトップは動作対象外であることを確認する（CON-2）
- [ ] SmartScreen / Defender の結果を記録する（CON-7）

## 計測・診断

- [ ] `config.MeasureHookLatency` 有効ビルドで callback 最大処理時間を記録し、悪化がない
- [ ] Enter送信ガードの検証完了後に `config.GuardTrace` を false へ戻す
