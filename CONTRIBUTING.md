# コントリビューション

## 開発環境

Windows 11 x64 に Go 1.26 以上、Node.js 22.12 以上、npm、Python 3.11 以上、公式 ChatGPT
デスクトップアプリを用意する。

```powershell
npm ci --ignore-scripts
npm run check
npm run release:check
```

アプリ本体、資格情報、アカウント状態、マスクされていないメールアドレスやデバイスコードを含む
キャプチャを commit してはならない。

## パッチの変更

レンダラーとメインプロセスのパッチは上流の正確なアンカーに依存する。変更は次を満たすこと。

1. 公式アプリを変更しないこと。
2. 期待するアンカーが存在しない場合に fail-closed で停止すること。
3. アカウントの隔離とスレッド所有の固定を維持すること。
4. 制御サービスをループバックとトークン認証に限定し続けること。
5. バックエンドの挙動には焦点を絞ったテストを追加し、利用者から見える新しい状態には
   必要に応じてスクリーンショットを添えること。

アンカーは公式ビルドごとの minify 結果に依存する。`docs/COMPATIBILITY.md` に記録された
ビルドで検証し、新しい公式ビルドでアンカーの変更が必要になった場合は同じ pull request で
その文書も更新する。

PowerShell スクリプトに非 ASCII を含める場合は UTF-8 BOM を付ける。Windows PowerShell 5.1 は
BOM が無いと ANSI として解釈する。`npm run check:powershell` がこれを検査する。

## fork 元の変更を取り込む

fork 元は macOS 版であり、本 fork とは大きく分岐している。`git merge upstream/main` は
使わず、コミット単位で選んで適用する。手順と判断基準は
[UPSTREAM-SYNC.md](docs/UPSTREAM-SYNC.md) にある。

```powershell
npm run upstream:check -- --fetch
```

## Pull Request

変更は焦点を絞り、セキュリティに影響する挙動は明示的に説明する。CI は Go のテストと vet、
JavaScript の構文、Python のコンパイル、PowerShell の構文と BOM、リリースメタデータの
整合性を検査する。
