// alt-ime-go は左右 Alt の空打ちで IME を OFF/ON する Windows 常駐ツール。
// 実装は internal/app にあり、このディレクトリはエントリポイントと埋め込み
// リソース (alt-ime.manifest / rsrc_windows_amd64.syso) を持つ。
//
// このファイルに build tag がないのは、GOOS!=windows のホストでも
// `go test ./...` / `go vet ./...` がパッケージを解決できるようにするため。
package main
