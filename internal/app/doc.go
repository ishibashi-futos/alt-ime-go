// Package app は alt-ime-go の Windows 専用実装（UI スレッド、フック専用
// スレッド、IME 送出、OSD、トレイ）を持つ。実装ファイルはすべて
// //go:build windows で、GOOS!=windows のホストではこのファイルだけが
// コンパイルされる（`go test ./...` / `go vet ./...` のパッケージ解決用）。
package app
