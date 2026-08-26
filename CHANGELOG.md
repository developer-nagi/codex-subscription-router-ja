# 変更履歴

本プロジェクトは [Keep a Changelog](https://keepachangelog.com/) に従い、
[セマンティックバージョニング](https://semver.org/lang/ja/) を採用する。

## [Unreleased]

### 変更

- 対象プラットフォームを macOS から **Windows 11 x64** へ全面的に置き換えた。公式アプリは
  MSIX パッケージ `OpenAI.Codex` を入力として自動検出する。
- 利用者から見えるすべての文字列を日本語化した。アカウントメニュー、利用枠の表示、
  サインイン導線、枯渇アラート、既定のアカウントラベルを含む。
- インストーラを `install.sh` から `install.ps1` へ置き換えた。
- ランチャーを macOS の C 実装から Go 実装 (`cmd/launcher`) へ置き換えた。
  `-H=windowsgui` でビルドし、コンソール窓を出さずに隔離プロファイルを渡す。
- CI とリリースのワークフローを `windows-latest` へ移行し、PowerShell の構文と BOM を
  検査する `npm run check:powershell` を追加した。

### 追加

- 多重化プロキシが `codex.real.exe` を解決し、Windows で子プロセスを終了できるようになった。
- 注入 UI がビルド依存の識別子をプレースホルダとして持ち、パッチャーが宣言の存在を検証して
  束縛するようになった。束縛できない場合は fail-closed で停止する。
- UI テストブリッジのセレクタを英語・日本語の両方に対応させた。
- プロフィール設定が合算であることを表示するようにした。接続が 2 件以上のとき、接続中の
  アカウントを重ねたアバターと「合算プロフィール」を示す。統計自体は以前から合算だったが、
  単一アカウントの識別情報を出していたため合算だと分からなかった。
- fork 元の変更を選択適用するための運用を整備した。`npm run upstream:check` が未確認の
  上流コミットを分類し、分岐元が古い上流ブランチを警告する。判断は
  `scripts/upstream-sync.json` に記録し、手順は `docs/UPSTREAM-SYNC.md` に置いた。
- 上流の Dependabot が提案する依存更新を個別に取り込んだ。`@electron/asar` 4.2.1 → 4.3.0、
  `actions/checkout` v6 → v7.0.1、`actions/setup-node` v6 → v7.0.0。上流の該当ブランチは
  リネーム前のコミットから分岐しているため、マージせず内容のみを適用した。

### 削除

- コード署名、Apple チーム識別子の書き換え、Computer Use ヘルパーの独立署名、
  `ElectronAsarIntegrity` の更新をすべて削除した。Windows ビルドは Electron fuse
  `EnableEmbeddedAsarIntegrityValidation` が無効で、これらの処理を必要としない。
- macOS 専用の `install.sh` と `native/launcher.c` を削除した。

### 上流から取り込み

- 外部プロバイダ経由のリクエストを合算 ChatGPT 利用枠の外へルーティングする
  (上流 PR #13)。独自プロバイダ、プロバイダ接頭辞付きモデル、OpenAI 以外のベース URL を
  検出し、そのアカウントの Codex プロバイダへそのまま送る。未マージ PR のため、上流で
  変更された場合は追随が必要。

### 上流から取り込み (続き)

- サブスクリプションの削除を追加した (上流 PR #21)。削除したアカウントの Codex ホームは
  `backups/accounts` へ退避し復元可能にする。Windows では終了していないプロセスが握る
  ディレクトリを移動できないため、子プロセスの終了を待ち、ハンドル解放を待って再試行する
  処理を加えた。
- `auth.json` のインポートを追加した (上流 PR #20)。アカウント側でデバイスコード認証を
  使えない場合の追加手段になる。
- 上流 PR #21 が取りこぼしていた CORS の `Access-Control-Allow-Headers` を復元し、
  preflight の回帰テストを追加した。欠けたままでは注入 UI からの通信が全て失敗する。
- 管理モードでは未接続のアカウントも表示するようにした。サインインに失敗したアカウントが
  UI から消せないままになるのを防ぐ。

### 未移植

- 設定 → プラグインのサブスクリプション切り替え、プロフィール設定のアカウント別アバター選択、
  スレッド要約の「サブスクリプション」欄、「残り利用枠」行から使用量モーダルを開く導線。
  Windows ビルドで該当画面の実装が変わったため。詳細は `docs/COMPATIBILITY.md` を参照。

## [0.1.0] - 2026-08-15

### 追加

- 利用枠を考慮した分散とスレッド固定によるマルチサブスクリプションルーティング。
- アカウントの隔離、デバイスコードサインイン、合算利用量、枯渇時のフェイルオーバー。
- ネイティブのアカウントメニュー、マスクされたメール、プラン表示、プロフィール写真。
- アカウント別選択に対応した合算プロフィール統計。
- 設定 → プラグインでのアカウント別 Apps および MCP 接続状態。
- アカウント別のレート制限リセット選択と、プール全体の枯渇処理。
- 独立署名による Appshots と Computer Use 対応。
- 上流互換性の fail-closed 検査と、深い階層から順に署名するヘルパー処理。
- ループバック限定でトークン認証された診断用 UI 状態。
- ソース限定の CI、リリース草稿の自動化、セキュリティ文書、実機確認手順。

[Unreleased]: https://github.com/developer-nagi/codex-subscription-router-win/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/developer-nagi/codex-subscription-router-win/releases/tag/v0.1.0
