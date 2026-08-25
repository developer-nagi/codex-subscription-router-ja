# リリース手順

リリースはソースのみを対象とする。パッチ済みアプリ、ASAR、抽出した公式ファイル、アカウント
データを添付してはならない。

1. `VERSION`、`package.json`、`package-lock.json` の両方のバージョン欄を更新する。
2. 変更履歴の Unreleased から `## [x.y.z] - YYYY-MM-DD` へ項目を移す。
3. 検証した公式アプリのバージョンと `app.asar` ハッシュを `docs/COMPATIBILITY.md` に記録する。
4. Windows 上で `npm ci --ignore-scripts`、`npm run check`、`npm run release:check` を実行する。
5. `docs/SMOKE-TEST.md` を完了し、対象コミットと Windows のバージョンをリリース草稿に記録する。
6. `git diff --check` を確認し、資格情報やアプリ本体が混入していないことを確かめる。
7. 保護された `release` 環境を設定し、確認済みコミットへ `vX.Y.Z` のタグを付けて push する。

リリースワークフローはタグと `VERSION` の一致を検証し、全チェックを再実行し、生成メモ付きの
ソース限定 GitHub リリース草稿を作成する。草稿と実機確認の記録を確認してから手動で公開する。
