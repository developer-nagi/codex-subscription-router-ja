# アーキテクチャ

Codex Subscription Router は、コピーしたアプリの `resources\codex.exe` を小さな Go 製多重化
プロキシへ置き換え、元のバイナリを同じディレクトリに `codex.real.exe` として保持する。

独立したデスクトップは `%APPDATA%\Codex Subscription Router` を使い、公式アプリの
`%APPDATA%\Codex` とは分離される。分離は 2 系統で行う。Electron の `app.setPath('userData')`
書き換えと、owl シェルの `resources\owl-app.ini` にある `UserDataDirectoryName` の変更である。
`.codex-mux` 状態ディレクトリは公式アプリと無関係に維持される。

## リクエストのルーティング

デスクトップは多重化プロキシへ JSON-RPC の app-server 接続を 1 本開く。多重化プロキシは有効な
アカウントごとに実際の app-server 子プロセスを起動し、それぞれに固有の `CODEX_HOME` と
`CODEX_SQLITE_HOME` を与える。

新しいスレッドは利用枠の逼迫度で割り当てる。週次の残り割合を、そのアカウントのリセットまでの
時間で割った値を基本スコアとし、貯まったリセットクレジットに上限付きの加点を与える。短期枠の
使用量、既存の固定スレッド数、安定したアカウント順で僅差を解消する。リセットクレジットの情報は
並列に取得し、5 分間キャッシュし、取得できない場合は中立として扱う。

スレッド ID が判明した時点で `state.json` に所有アカウントを永続化する。リクエスト、レスポンス、
承認、通知は、1 つの一貫したデスクトップセッションを保つために必要な範囲でのみ書き換える。

所有アカウントが枯渇した場合は、余力のあるアカウントでロールアウトを再開し所有権を更新する。
通常の負荷分散のためにスレッドを移動させることはない。

## 素通し

多重化プロキシは `app-server` サブコマンドの対話起動のみを横取りする。`app-server daemon`、
`app-server proxy`、スキーマ生成、その他のサブコマンドは `codex.real.exe` へそのまま渡し、
終了コードを保持する。実バイナリは `codex.real.exe`、次に `codex.real` の順で解決する。

## アカウントの隔離

プライマリアカウントは `%USERPROFILE%\.codex` を使う。追加したアカウントは
`%USERPROFILE%\.codex-mux\accounts\<id>\codex-home` を使う。管理対象の構成はプライマリから
コピーするが、資格情報ストア設定とプロジェクトの信頼設定は除外する。各隔離アカウントは
CLI と MCP の OAuth 資格情報をファイル保存へ強制する。

公式のプラグインおよびスキルはユーザープロファイル配下の `.agents` に置かれ、`CODEX_HOME` の
外にあるため全アカウントで共有される。

## デスクトップへの組み込み

パッチャーは `app.asar` を展開し、上流のアンカーが一意に一致することを検証し、アカウント UI を
挿入し、アーカイブを再パックする。Windows ビルドは Electron fuse
`EnableEmbeddedAsarIntegrityValidation` が無効のため、整合性ハッシュの更新もコード署名も不要である。
ネイティブモジュール (`@worklouder`、`better-sqlite3`、`node-pty`) は展開状態で保持し、
再パック後に検証する。

`CodexSubscriptionRouter.exe` は Go 製のランチャーで、`-H=windowsgui` でビルドされコンソール窓を
出さない。Electron のメインプロセスが動き出す前に、隔離したプロファイルを `--user-data-dir` として
渡す。

Computer Use は Windows では `resources\cua_node` の Node ランタイムと `resources\native` の
ネイティブモジュールを直接使う。macOS のような独立署名ヘルパーアプリやプライバシー許可の
仕組みは存在しないため、対応するパッチも持たない。

## 制御 API

レンダラーはポート 48123 のループバック限定 HTTP サービスと通信する。非公開経路はすべて
ランダムな 256 ビットトークンを要求する。CORS はコピーしたアプリの `app://-` オリジンに限定する。
このサービスはアカウント情報、集計した利用量とプロフィール、スレッド所有、サインインとサインアウト、
認証付き SSE イベントストリームを提供し、OAuth トークンを返すことはない。
