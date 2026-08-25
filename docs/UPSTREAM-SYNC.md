# fork 元の変更を取り込む

本 fork は fork 元 (`b-nnett/codex-subscription-router`) を macOS 版から Windows 版へ
全面的に置き換え、利用者から見える文字列を日本語化している。共有しているのは Go の
多重化ロジックと注入 UI の一部だけで、パッチャー、インストーラ、CI、文書は別物である。

そのため **`git merge upstream/main` は使わない**。上流の変更はコミット単位で確認し、
必要なものだけを適用する。

## 原則

1. 上流ブランチを直接 merge しない。分岐元が古いブランチを merge すると、その後に
   入った上流の変更を巻き戻す。
2. 適用するかどうかの判断と根拠を `scripts/upstream-sync.json` に残す。同じ検討を
   繰り返さないため。
3. 適用したら `npm run check` と、パッチャーに影響する変更なら実機での再ビルドまで
   確認する。

## 確認する

```powershell
npm run upstream:check
```

`upstream` remote を fetch してから確認する場合:

```powershell
npm run upstream:check -- --fetch
```

出力は未確認の上流コミットを列挙し、変更ファイルを次のように分類する。

| 分類 | 意味 | 扱い |
| --- | --- | --- |
| 取り込み候補 | `cmd/`、`internal/`、`ui/`、`go.mod`、`VERSION` | 本 fork と共有。多くはそのまま適用できる |
| 要注意 | `scripts/`、`README.md`、`docs/`、`.github/workflows/`、ルート文書 | Windows 向けに書き換え済み。差分を読み、必要なら手で作り直す |
| 対象外 | `install.sh`、`native/` | 本 fork では削除済み。適用しない |
| 分類なし | 上記以外 | 個別に判断し、必要なら `scripts/check_upstream.py` の分類を更新する |

分岐元が上流の先端より古いブランチは、警告として併記される。

## 適用する

### 共有部分 (`cmd/`、`internal/`、`ui/`)

作業ツリーを綺麗にしてから、コミットせずに取り込んで内容を確認する。

```bash
git cherry-pick -n <commit>
```

衝突したら、本 fork 側の Windows 対応と日本語化を保ったまま解決する。特に次は
巻き戻さないよう注意する。

- Go のモジュールパス (`github.com/developer-nagi/codex-subscription-router-ja`)
- 日本語のユーザー向け文字列 (`internal/mux/mux.go`、`internal/state/store.go`)
- Windows 固有の分岐 (`internal/backend/terminate_windows.go`、`resolveRealExecutable`)

### 書き換え済み部分 (`scripts/`、`docs/`、ワークフロー)

cherry-pick せず、差分を読んで意図だけを取り込む。

```bash
git show <commit> -- scripts/ docs/
```

パッチャーのアンカーに関する上流の変更は、macOS ビルドを前提にしている。Windows
ビルドでは識別子が異なるため、そのまま持ち込まない。`docs/COMPATIBILITY.md` の
アンカー表と識別子束縛表を実ビルドに対して再確認する。

### 依存更新 (Dependabot)

上流の Dependabot ブランチは merge せず、バージョン指定だけを適用する。

```powershell
npm install --save-exact --ignore-scripts @electron/asar@<version>
```

GitHub Actions は SHA で固定しているため、SHA を差し替える。値は必ず検証する。

```bash
gh api repos/actions/checkout/commits/<sha> --jq .sha
```

`@electron/asar` はパッチャーの中核 (extract / pack / list) なので、更新したら実機で
再ビルドまで確認する。

## 記録する

適用または見送りを決めたら、`scripts/upstream-sync.json` の `decisions` に追記し、
確認済みコミットを進める。

```powershell
npm run upstream:check -- --mark-reviewed
```

特定のコミットまでを確認済みにする場合は、コミットを渡す。

```powershell
npm run upstream:check -- --mark-reviewed <commit>
```

## 検証する

```powershell
npm run check
npm run release:check
```

パッチャー、注入 UI、Go の多重化ロジックに触れた場合は、実機での再ビルドと起動確認まで
行う。手順は [SMOKE-TEST.md](SMOKE-TEST.md) にある。

```powershell
python scripts/patch_app.py --force
```

## これまでの判断

`scripts/upstream-sync.json` を参照する。2026-08-26 時点では、上流 `main` に本 fork の
分岐後の新規コミットはなく、Dependabot ブランチ 3 本のうち 2 本が先端より 3 コミット前から
分岐していたため merge せず、内容のみを適用している。
