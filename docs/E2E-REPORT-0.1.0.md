# 実機検証レポート — Windows 移植

対象コミット: Windows 移植作業ブランチ
実施環境: Windows 11 Pro 10.0.26200 (x64)
公式アプリ: MSIX `OpenAI.Codex` `26.818.8289.0`
`app.asar` SHA-256: `e2f04d6aa921d07981b42368df0a28a8bebe8cd21375d4a1f9286757b51c1313`

## 前提の検証

| 確認項目 | 結果 |
| --- | --- |
| 公式アプリの実体 | MSIX パッケージ。内部は Electron + `app.asar` (287 MB) |
| MSIX 外へコピーして起動 | 成功。ウィンドウ表示と子プロセス 11 個を確認 |
| コード署名・パッケージ ID | 不要。コピーしたアプリはそのまま起動する |
| Electron fuse `EnableEmbeddedAsarIntegrityValidation` | 無効。再パック時のハッシュ更新が不要 |
| 起動される app-server | `resources\codex.exe -c features.code_mode_host=true app-server --analytics-default-enabled` |

## パッチアンカー

`docs/COMPATIBILITY.md` に列挙した 16 箇所すべてが、公式ビルドに対して一意 (一致 1 件) で
あることを自動検査で確認した。

## ビルドとインストール

`python scripts/patch_app.py` が終了コード 0 で完了し、次を作成した。

- `%LOCALAPPDATA%\Programs\Codex Subscription Router`
- スタートメニューのショートカット

インストール後の状態:

| 確認項目 | 結果 |
| --- | --- |
| `resources\codex.exe` | 多重化プロキシ (7.2 MB) に置換 |
| `resources\codex.real.exe` | 公式バイナリ (297 MB) を保持 |
| `resources\codex.exe --help` | 公式 CLI のヘルプを表示 (素通し成功) |
| `owl-app.ini` の `UserDataDirectoryName` | `Codex Subscription Router` |
| `app.asar.unpacked\node_modules` | `@worklouder`、`better-sqlite3`、`node-pty` を保持 |
| 公式 `app.asar` のハッシュ | パッチ前と同一 (公式アプリは無変更) |

## 起動時の動作

| 確認項目 | 結果 |
| --- | --- |
| ランチャーからの起動 | 成功。プロセス 12 個、ウィンドウ表示 |
| 多重化プロキシの起動 | `resources\codex.exe ... app-server` として起動 |
| 実 app-server 子プロセス | `codex.real.exe ... app-server` を起動 |
| `/v1/health` | `200 {"ok":true}` |
| `/v1/accounts` (トークン認証) | アカウント 1 件を返却。`connected=true`、`planLabel=Pro 20x` |
| 日本語ラベル | `"label":"プライマリ"` を正しい UTF-8 で返却 |
| スレッド所有の永続化 | `state.json` に記録を確認 |
| 公式アプリへの影響 | 同時起動中の公式アプリは正常動作を維持 |

## レンダラーの描画

| 確認項目 | 結果 |
| --- | --- |
| アプリ画面の描画 | 正常。日本語 UI で完全に描画される |
| レンダラーの JavaScript エラー | なし。診断に残るのは i18n の欠落警告とルーターの情報ログのみ |
| 注入したアカウントメニュー | プロフィールメニュー内に描画。「残り利用枠 97%」「プライマリ · Pro 20x 97%」「サブスクリプションを追加」を確認 |
| メールアドレスのマスク | ホバー前はマスク表示 |

注入 UI が参照する識別子は `docs/COMPATIBILITY.md` の束縛表のとおり、公式ビルドの宣言を
検証したうえで置換している。macOS 版の識別子をそのまま注入するとレンダラーが起動時に落ちる。

## 未実施

- サブスクリプションを 2 件以上接続した状態でのルーティング、フェイルオーバー、
  アカウント別リセットの確認。実施には追加の ChatGPT サブスクリプションが必要。
- `docs/COMPATIBILITY.md` に記載した未移植機能の確認。
